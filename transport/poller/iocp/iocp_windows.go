//go:build windows

package iocp

import (
	"errors"
	"sync"
	"unsafe"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport/poller"
	"golang.org/x/sys/windows"
)

const acceptAddressLength = 128

type pendingRequest struct {
	req     poller.IORequest
	accept  *acceptContext
	wsabufs []windows.WSABuf
	from    windows.RawSockaddrAny
	fromLen int32
	to      windows.RawSockaddrAny
	toLen   int32
}

type acceptContext struct {
	buf [acceptAddressLength * 2]byte
}

type Poller struct {
	port windows.Handle

	mu      sync.Mutex
	closed  bool
	entries map[poller.FDRef]poller.ChannelID
	pending map[*windows.Overlapped]*pendingRequest
}

func New() (poller.Poller, error) {
	port, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	return &Poller{
		port:    port,
		entries: make(map[poller.FDRef]poller.ChannelID, 1024),
		pending: make(map[*windows.Overlapped]*pendingRequest, 1024),
	}, nil
}

func (p *Poller) Model() poller.Model {
	return poller.Completion
}

func (p *Poller) Backend() poller.BackendKind {
	return poller.BackendIOCP
}

func (p *Poller) Register(fd poller.FDRef, ch poller.ChannelID, _ poller.ReadyMask) error {
	if !fd.Valid() {
		return poller.ErrInvalidFD
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return poller.ErrClosedPoller
	}
	_, err := windows.CreateIoCompletionPort(windows.Handle(uintptr(fd.FD)), p.port, uintptr(ch), 0)
	if err != nil {
		return err
	}
	p.entries[fd] = ch
	return nil
}

func (p *Poller) Modify(fd poller.FDRef, _ poller.ReadyMask) error {
	if !fd.Valid() {
		return poller.ErrInvalidFD
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return poller.ErrClosedPoller
	}
	return nil
}

func (p *Poller) Deregister(fd poller.FDRef) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, fd)
	return nil
}

func (p *Poller) Submit(req poller.IORequest) error {
	if req.Op == poller.OpWakeup {
		return p.Wakeup()
	}
	if !validRequest(req) {
		return poller.ErrInvalidIORequest
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return poller.ErrClosedPoller
	}
	ov := &windows.Overlapped{}
	if req.Buf != nil {
		req.Buf.Retain()
	}
	for _, buf := range req.Bufs {
		buf.Retain()
	}
	pending := &pendingRequest{req: req}
	if req.Op == poller.OpAccept {
		pending.accept = &acceptContext{}
	}
	if req.Op == poller.OpRead && req.Datagram {
		pending.fromLen = int32(unsafe.Sizeof(pending.from))
	}
	if req.Op == poller.OpWrite {
		wsabufs, err := makeWriteBuffers(req)
		if err != nil {
			if req.Buf != nil {
				req.Buf.Release()
			}
			for _, buf := range req.Bufs {
				buf.Release()
			}
			p.mu.Unlock()
			return err
		}
		pending.wsabufs = wsabufs
		if req.Datagram {
			to, toLen, err := makeRawSockaddr(req.Addr)
			if err != nil {
				if req.Buf != nil {
					req.Buf.Release()
				}
				for _, buf := range req.Bufs {
					buf.Release()
				}
				p.mu.Unlock()
				return err
			}
			pending.to = to
			pending.toLen = toLen
		}
	}
	p.pending[ov] = pending
	p.mu.Unlock()

	var err error
	switch req.Op {
	case poller.OpAccept:
		err = p.submitAccept(req, ov, pending.accept)
	case poller.OpRead:
		err = p.submitRead(req, ov, pending)
	case poller.OpWrite:
		err = p.submitWrite(req, ov, pending.wsabufs, pending)
	case poller.OpClose:
		err = p.submitClose(req, ov)
	default:
		err = poller.ErrInvalidIORequest
	}
	if err == nil || err == windows.ERROR_IO_PENDING {
		return nil
	}

	p.mu.Lock()
	delete(p.pending, ov)
	p.mu.Unlock()
	if req.Buf != nil {
		req.Buf.Release()
	}
	for _, buf := range req.Bufs {
		buf.Release()
	}
	if req.Op == poller.OpAccept && req.AcceptedFD.Valid() {
		_ = windows.Closesocket(windows.Handle(uintptr(req.AcceptedFD.FD)))
	}
	return err
}

func (p *Poller) Poll(dst []poller.Event, timeoutMillis int) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	timeout := uint32(timeoutMillis)
	if timeoutMillis < 0 {
		timeout = windows.INFINITE
	}
	out := 0
	for out < len(dst) {
		var transferred uint32
		var key uintptr
		var ov *windows.Overlapped
		err := windows.GetQueuedCompletionStatus(p.port, &transferred, &key, &ov, timeout)
		timeout = 0
		if err == windows.Errno(windows.WAIT_TIMEOUT) {
			break
		}
		if ov == nil && key == 0 {
			if err != nil {
				return out, err
			}
			dst[out] = poller.Event{Model: poller.Completion, Op: poller.OpWakeup}
			out++
			continue
		}
		pending := p.takePending(ov)
		if pending == nil {
			continue
		}
		req := pending.req
		if req.Buf != nil && req.Op == poller.OpRead && transferred > 0 {
			if advErr := req.Buf.AdvanceWriter(int(transferred)); advErr != nil && err == nil {
				err = advErr
			}
		}
		addr := req.Addr
		if req.Op == poller.OpRead && req.Datagram && err == nil {
			addr = socketAddressFromRaw(&pending.from, pending.fromLen)
		}
		dst[out] = poller.Event{
			Model:      poller.Completion,
			Op:         req.Op,
			Ready:      poller.CompletionReady(req.Op),
			FD:         req.FD,
			AcceptedFD: req.AcceptedFD,
			ChannelID:  req.ChannelID,
			OpID:       req.OpID,
			Buf:        req.Buf,
			Bufs:       req.Bufs,
			Addr:       addr,
			N:          int(transferred),
			Err:        err,
		}
		out++
	}
	return out, nil
}

func (p *Poller) Wakeup() error {
	return windows.PostQueuedCompletionStatus(p.port, 0, 0, nil)
}

func (p *Poller) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	for ov, pending := range p.pending {
		delete(p.pending, ov)
		req := pending.req
		cancelPending(req, ov)
		if req.Buf != nil {
			req.Buf.Release()
		}
		for _, buf := range req.Bufs {
			buf.Release()
		}
		if req.Op == poller.OpAccept && req.AcceptedFD.Valid() {
			_ = windows.Closesocket(windows.Handle(uintptr(req.AcceptedFD.FD)))
		}
	}
	p.mu.Unlock()
	return windows.CloseHandle(p.port)
}

func cancelPending(req poller.IORequest, ov *windows.Overlapped) {
	if ov == nil || !req.FD.Valid() {
		return
	}
	err := windows.CancelIoEx(windows.Handle(uintptr(req.FD.FD)), ov)
	if err != nil && !errors.Is(err, windows.ERROR_NOT_FOUND) && !errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
		return
	}
}

func (p *Poller) submitAccept(req poller.IORequest, ov *windows.Overlapped, ctx *acceptContext) error {
	if ctx == nil {
		return poller.ErrInvalidIORequest
	}
	var recvd uint32
	return windows.AcceptEx(
		windows.Handle(uintptr(req.FD.FD)),
		windows.Handle(uintptr(req.AcceptedFD.FD)),
		&ctx.buf[0],
		0,
		acceptAddressLength,
		acceptAddressLength,
		&recvd,
		ov,
	)
}

func (p *Poller) submitRead(req poller.IORequest, ov *windows.Overlapped, pending *pendingRequest) error {
	view := req.Buf.WritableBytesView()
	if len(view) == 0 {
		return buffer.ErrNoWritableBytes
	}
	wsabuf := windows.WSABuf{Len: uint32(len(view)), Buf: &view[0]}
	var flags uint32
	var recvd uint32
	if req.Datagram {
		if pending == nil {
			return poller.ErrInvalidIORequest
		}
		return windows.WSARecvFrom(windows.Handle(uintptr(req.FD.FD)), &wsabuf, 1, &recvd, &flags, &pending.from, &pending.fromLen, ov, nil)
	}
	return windows.WSARecv(windows.Handle(uintptr(req.FD.FD)), &wsabuf, 1, &recvd, &flags, ov, nil)
}

func (p *Poller) submitWrite(req poller.IORequest, ov *windows.Overlapped, wsabufs []windows.WSABuf, pending *pendingRequest) error {
	if len(wsabufs) == 0 {
		return poller.ErrInvalidIORequest
	}
	var sent uint32
	if req.Datagram {
		if pending == nil || pending.toLen == 0 {
			return poller.ErrInvalidIORequest
		}
		return windows.WSASendTo(windows.Handle(uintptr(req.FD.FD)), &wsabufs[0], uint32(len(wsabufs)), &sent, 0, &pending.to, pending.toLen, ov, nil)
	}
	return windows.WSASend(windows.Handle(uintptr(req.FD.FD)), &wsabufs[0], uint32(len(wsabufs)), &sent, 0, ov, nil)
}

func makeWriteBuffers(req poller.IORequest) ([]windows.WSABuf, error) {
	if req.Buf != nil {
		data := req.Buf.Bytes()
		if len(data) == 0 {
			return nil, poller.ErrInvalidIORequest
		}
		return []windows.WSABuf{{Len: uint32(len(data)), Buf: &data[0]}}, nil
	}
	wsabufs := make([]windows.WSABuf, 0, len(req.Bufs))
	for _, buf := range req.Bufs {
		slices := buf.ReadableSlices(nil)
		for _, data := range slices {
			if len(data) == 0 {
				continue
			}
			wsabufs = append(wsabufs, windows.WSABuf{Len: uint32(len(data)), Buf: &data[0]})
		}
	}
	if len(wsabufs) == 0 {
		return nil, poller.ErrInvalidIORequest
	}
	return wsabufs, nil
}

func (p *Poller) submitClose(req poller.IORequest, ov *windows.Overlapped) error {
	err := windows.Closesocket(windows.Handle(uintptr(req.FD.FD)))
	if err != nil {
		return err
	}
	return windows.PostQueuedCompletionStatus(p.port, 0, uintptr(req.ChannelID), ov)
}

func (p *Poller) takePending(ov *windows.Overlapped) *pendingRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	req := p.pending[ov]
	delete(p.pending, ov)
	return req
}

func validRequest(req poller.IORequest) bool {
	switch req.Op {
	case poller.OpAccept:
		return req.FD.Valid() && req.AcceptedFD.Valid()
	case poller.OpClose:
		return req.FD.Valid()
	case poller.OpRead, poller.OpWrite:
		if !req.FD.Valid() || (req.Buf == nil && len(req.Bufs) == 0) {
			return false
		}
		return !req.Datagram || req.Op == poller.OpRead || req.Addr.Valid()
	default:
		return false
	}
}

func makeRawSockaddr(addr poller.SocketAddress) (windows.RawSockaddrAny, int32, error) {
	var rsa windows.RawSockaddrAny
	switch addr.Family {
	case poller.SocketFamilyIPv4:
		raw := (*windows.RawSockaddrInet4)(unsafe.Pointer(&rsa))
		raw.Family = windows.AF_INET
		raw.Port = htons(uint16(addr.Port))
		copy(raw.Addr[:], addr.IP[:4])
		return rsa, int32(unsafe.Sizeof(*raw)), nil
	case poller.SocketFamilyIPv6:
		raw := (*windows.RawSockaddrInet6)(unsafe.Pointer(&rsa))
		raw.Family = windows.AF_INET6
		raw.Port = htons(uint16(addr.Port))
		raw.Scope_id = addr.ZoneID
		copy(raw.Addr[:], addr.IP[:])
		return rsa, int32(unsafe.Sizeof(*raw)), nil
	default:
		return rsa, 0, poller.ErrInvalidIORequest
	}
}

func socketAddressFromRaw(rsa *windows.RawSockaddrAny, n int32) poller.SocketAddress {
	if rsa == nil || n <= 0 {
		return poller.SocketAddress{}
	}
	switch rsa.Addr.Family {
	case windows.AF_INET:
		raw := (*windows.RawSockaddrInet4)(unsafe.Pointer(rsa))
		var addr poller.SocketAddress
		addr.Family = poller.SocketFamilyIPv4
		addr.Port = int(ntohs(raw.Port))
		copy(addr.IP[:4], raw.Addr[:])
		return addr
	case windows.AF_INET6:
		raw := (*windows.RawSockaddrInet6)(unsafe.Pointer(rsa))
		var addr poller.SocketAddress
		addr.Family = poller.SocketFamilyIPv6
		addr.Port = int(ntohs(raw.Port))
		addr.ZoneID = raw.Scope_id
		copy(addr.IP[:], raw.Addr[:])
		return addr
	default:
		return poller.SocketAddress{}
	}
}

func htons(v uint16) uint16 {
	return v<<8 | v>>8
}

func ntohs(v uint16) uint16 {
	return htons(v)
}

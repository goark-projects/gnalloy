//go:build windows

package iocp

import (
	"sync"

	"github.com/goark-projects/gnalloy/buffer"
	"github.com/goark-projects/gnalloy/transport/poller"
	"golang.org/x/sys/windows"
)

type Poller struct {
	port windows.Handle

	mu      sync.Mutex
	closed  bool
	entries map[poller.FDRef]poller.ChannelID
	pending map[*windows.Overlapped]poller.IORequest
}

func New() (poller.Poller, error) {
	port, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	return &Poller{
		port:    port,
		entries: make(map[poller.FDRef]poller.ChannelID, 1024),
		pending: make(map[*windows.Overlapped]poller.IORequest, 1024),
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
	if !req.FD.Valid() || req.Buf == nil {
		return poller.ErrInvalidIORequest
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return poller.ErrClosedPoller
	}
	ov := &windows.Overlapped{}
	req.Buf.Retain()
	p.pending[ov] = req
	p.mu.Unlock()

	var err error
	switch req.Op {
	case poller.OpRead:
		err = p.submitRead(req, ov)
	case poller.OpWrite:
		err = p.submitWrite(req, ov)
	default:
		err = poller.ErrInvalidIORequest
	}
	if err == nil || err == windows.ERROR_IO_PENDING {
		return nil
	}

	p.mu.Lock()
	delete(p.pending, ov)
	p.mu.Unlock()
	req.Buf.Release()
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
		req := p.takePending(ov)
		if req.Buf != nil && req.Op == poller.OpRead && transferred > 0 {
			if advErr := req.Buf.AdvanceWriter(int(transferred)); advErr != nil && err == nil {
				err = advErr
			}
		}
		dst[out] = poller.Event{
			Model:     poller.Completion,
			Op:        req.Op,
			Ready:     poller.CompletionReady(req.Op),
			FD:        req.FD,
			ChannelID: req.ChannelID,
			OpID:      req.OpID,
			Buf:       req.Buf,
			N:         int(transferred),
			Err:       err,
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
	for ov, req := range p.pending {
		delete(p.pending, ov)
		if req.Buf != nil {
			req.Buf.Release()
		}
	}
	p.mu.Unlock()
	return windows.CloseHandle(p.port)
}

func (p *Poller) submitRead(req poller.IORequest, ov *windows.Overlapped) error {
	view := req.Buf.WritableBytesView()
	if len(view) == 0 {
		return buffer.ErrNoWritableBytes
	}
	wsabuf := windows.WSABuf{Len: uint32(len(view)), Buf: &view[0]}
	var flags uint32
	var recvd uint32
	return windows.WSARecv(windows.Handle(uintptr(req.FD.FD)), &wsabuf, 1, &recvd, &flags, ov, nil)
}

func (p *Poller) submitWrite(req poller.IORequest, ov *windows.Overlapped) error {
	data := req.Buf.Bytes()
	if len(data) == 0 {
		return poller.ErrInvalidIORequest
	}
	wsabuf := windows.WSABuf{Len: uint32(len(data)), Buf: &data[0]}
	var sent uint32
	return windows.WSASend(windows.Handle(uintptr(req.FD.FD)), &wsabuf, 1, &sent, 0, ov, nil)
}

func (p *Poller) takePending(ov *windows.Overlapped) poller.IORequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	req := p.pending[ov]
	delete(p.pending, ov)
	return req
}

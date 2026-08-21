//go:build linux

package iouring

import (
	"errors"
	"sync/atomic"
	"unsafe"

	"github.com/goark-projects/gnalloy/transport/poller"
	"golang.org/x/sys/unix"
)

const (
	defaultEntries = 256

	offSqRing = 0
	offCqRing = 0x8000000
	offSqes   = 0x10000000

	enterGetEvents = 1

	featSingleMmap = 1

	opNop   = 0
	opRead  = 22
	opWrite = 23
)

type sqringOffsets struct {
	head        uint32
	tail        uint32
	ringMask    uint32
	ringEntries uint32
	flags       uint32
	dropped     uint32
	array       uint32
	resv1       uint32
	userAddr    uint64
}

type cqringOffsets struct {
	head        uint32
	tail        uint32
	ringMask    uint32
	ringEntries uint32
	overflow    uint32
	cqes        uint32
	flags       uint32
	resv1       uint32
	userAddr    uint64
}

type params struct {
	sqEntries    uint32
	cqEntries    uint32
	flags        uint32
	sqThreadCPU  uint32
	sqThreadIdle uint32
	features     uint32
	wqFD         uint32
	resv         [3]uint32
	sqOff        sqringOffsets
	cqOff        cqringOffsets
}

type sqe struct {
	opcode      uint8
	flags       uint8
	ioprio      uint16
	fd          int32
	off         uint64
	addr        uint64
	len         uint32
	rwFlags     uint32
	userData    uint64
	bufIndex    uint16
	personality uint16
	spliceFdIn  int32
	addr3       uint64
	pad2        [1]uint64
}

type cqe struct {
	userData uint64
	res      int32
	flags    uint32
}

type sq struct {
	ring        []byte
	sqes        []byte
	head        *uint32
	tail        *uint32
	ringMask    *uint32
	ringEntries *uint32
	array       *uint32
}

type cq struct {
	ring     []byte
	head     *uint32
	tail     *uint32
	ringMask *uint32
	cqes     *cqe
}

type Poller struct {
	fd int

	params params
	sq     sq
	cq     cq

	entries map[poller.FDRef]poller.ChannelID
	pending map[uint64]poller.IORequest
	nextID  uint64
	closed  bool
}

func New() (poller.Poller, error) {
	return NewWithEntries(defaultEntries)
}

func NewWithEntries(entries uint32) (poller.Poller, error) {
	if entries == 0 {
		entries = defaultEntries
	}
	p := &Poller{
		entries: make(map[poller.FDRef]poller.ChannelID, entries),
		pending: make(map[uint64]poller.IORequest, entries),
	}
	if err := p.setup(entries); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Poller) Model() poller.Model {
	return poller.Completion
}

func (p *Poller) Backend() poller.BackendKind {
	return poller.BackendIOUring
}

func (p *Poller) Register(fd poller.FDRef, ch poller.ChannelID, _ poller.ReadyMask) error {
	if !fd.Valid() {
		return poller.ErrInvalidFD
	}
	if p.closed {
		return poller.ErrClosedPoller
	}
	p.entries[fd] = ch
	return nil
}

func (p *Poller) Modify(fd poller.FDRef, _ poller.ReadyMask) error {
	if !fd.Valid() {
		return poller.ErrInvalidFD
	}
	if p.closed {
		return poller.ErrClosedPoller
	}
	return nil
}

func (p *Poller) Deregister(fd poller.FDRef) error {
	delete(p.entries, fd)
	return nil
}

func (p *Poller) Submit(req poller.IORequest) error {
	if p.closed {
		return poller.ErrClosedPoller
	}
	if req.Op != poller.OpWakeup && (!req.FD.Valid() || req.Buf == nil) {
		return poller.ErrInvalidIORequest
	}
	id := req.OpID
	if id == 0 {
		id = poller.OpID(p.nextUserData())
		req.OpID = id
	}
	if req.Buf != nil {
		req.Buf.Retain()
	}
	if err := p.prepare(uint64(id), req); err != nil {
		if req.Buf != nil {
			req.Buf.Release()
		}
		return err
	}
	p.pending[uint64(id)] = req
	return p.enter(1, 0, 0)
}

func (p *Poller) Poll(dst []poller.Event, timeoutMillis int) (int, error) {
	if p.closed {
		return 0, poller.ErrClosedPoller
	}
	if len(dst) == 0 {
		return 0, nil
	}
	n := p.reap(dst)
	if n > 0 || timeoutMillis == 0 {
		return n, nil
	}
	if timeoutMillis > 0 {
		pollfd := []unix.PollFd{{Fd: int32(p.fd), Events: unix.POLLIN}}
		ready, err := unix.Poll(pollfd, timeoutMillis)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				return 0, nil
			}
			return 0, err
		}
		if ready == 0 {
			return 0, nil
		}
	} else if err := p.enter(0, 1, enterGetEvents); err != nil {
		if errors.Is(err, unix.EINTR) {
			return 0, nil
		}
		return 0, err
	}
	return p.reap(dst), nil
}

func (p *Poller) Wakeup() error {
	return p.Submit(poller.IORequest{Op: poller.OpWakeup})
}

func (p *Poller) Close() error {
	if p.closed {
		return nil
	}
	p.closed = true
	for _, req := range p.pending {
		if req.Buf != nil {
			req.Buf.Release()
		}
	}
	p.pending = nil
	err1 := unmapIfPresent(p.sq.sqes)
	if len(p.cq.ring) > 0 && len(p.sq.ring) > 0 && &p.cq.ring[0] == &p.sq.ring[0] {
		p.cq.ring = nil
	}
	err2 := unmapIfPresent(p.cq.ring)
	err3 := unmapIfPresent(p.sq.ring)
	err4 := unix.Close(p.fd)
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	if err3 != nil {
		return err3
	}
	return err4
}

func (p *Poller) setup(entries uint32) error {
	fd, _, errno := unix.Syscall(unix.SYS_IO_URING_SETUP, uintptr(entries), uintptr(unsafe.Pointer(&p.params)), 0)
	if errno != 0 {
		return errno
	}
	p.fd = int(fd)

	sqRingSize := p.params.sqOff.array + p.params.sqEntries*uint32(unsafe.Sizeof(uint32(0)))
	cqRingSize := p.params.cqOff.cqes + p.params.cqEntries*uint32(unsafe.Sizeof(cqe{}))
	if p.params.features&featSingleMmap != 0 && cqRingSize > sqRingSize {
		sqRingSize = cqRingSize
	}

	sqRing, err := unix.Mmap(p.fd, offSqRing, int(sqRingSize), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_POPULATE)
	if err != nil {
		_ = unix.Close(p.fd)
		return err
	}
	cqRing := sqRing
	if p.params.features&featSingleMmap == 0 {
		cqRing, err = unix.Mmap(p.fd, offCqRing, int(cqRingSize), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_POPULATE)
		if err != nil {
			_ = unix.Munmap(sqRing)
			_ = unix.Close(p.fd)
			return err
		}
	}
	sqesSize := int(p.params.sqEntries) * int(unsafe.Sizeof(sqe{}))
	sqes, err := unix.Mmap(p.fd, offSqes, sqesSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_POPULATE)
	if err != nil {
		if p.params.features&featSingleMmap == 0 {
			_ = unix.Munmap(cqRing)
		}
		_ = unix.Munmap(sqRing)
		_ = unix.Close(p.fd)
		return err
	}

	p.sq = sq{
		ring:        sqRing,
		sqes:        sqes,
		head:        uint32Ptr(sqRing, p.params.sqOff.head),
		tail:        uint32Ptr(sqRing, p.params.sqOff.tail),
		ringMask:    uint32Ptr(sqRing, p.params.sqOff.ringMask),
		ringEntries: uint32Ptr(sqRing, p.params.sqOff.ringEntries),
		array:       uint32Ptr(sqRing, p.params.sqOff.array),
	}
	p.cq = cq{
		ring:     cqRing,
		head:     uint32Ptr(cqRing, p.params.cqOff.head),
		tail:     uint32Ptr(cqRing, p.params.cqOff.tail),
		ringMask: uint32Ptr(cqRing, p.params.cqOff.ringMask),
		cqes:     (*cqe)(unsafe.Pointer(&cqRing[p.params.cqOff.cqes])),
	}
	return nil
}

func (p *Poller) prepare(userData uint64, req poller.IORequest) error {
	head := atomic.LoadUint32(p.sq.head)
	tail := atomic.LoadUint32(p.sq.tail)
	if tail-head >= atomic.LoadUint32(p.sq.ringEntries) {
		return poller.ErrInvalidIORequest
	}
	index := tail & atomic.LoadUint32(p.sq.ringMask)
	entry := p.sqe(index)
	*entry = sqe{}
	entry.userData = userData

	switch req.Op {
	case poller.OpWakeup:
		entry.opcode = opNop
	case poller.OpRead:
		view := req.Buf.WritableBytesView()
		if len(view) == 0 {
			return poller.ErrInvalidIORequest
		}
		entry.opcode = opRead
		entry.fd = int32(req.FD.FD)
		entry.addr = uint64(uintptr(unsafe.Pointer(&view[0])))
		entry.len = uint32(len(view))
	case poller.OpWrite:
		data := req.Buf.Bytes()
		if len(data) == 0 {
			return poller.ErrInvalidIORequest
		}
		entry.opcode = opWrite
		entry.fd = int32(req.FD.FD)
		entry.addr = uint64(uintptr(unsafe.Pointer(&data[0])))
		entry.len = uint32(len(data))
	default:
		return poller.ErrInvalidIORequest
	}

	array := (*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(p.sq.array)) + uintptr(index)*unsafe.Sizeof(uint32(0))))
	atomic.StoreUint32(array, index)
	atomic.StoreUint32(p.sq.tail, tail+1)
	return nil
}

func (p *Poller) reap(dst []poller.Event) int {
	head := atomic.LoadUint32(p.cq.head)
	tail := atomic.LoadUint32(p.cq.tail)
	out := 0
	for head != tail && out < len(dst) {
		index := head & atomic.LoadUint32(p.cq.ringMask)
		cqe := p.cqe(index)
		req := p.pending[cqe.userData]
		delete(p.pending, cqe.userData)

		n := int(cqe.res)
		var err error
		if cqe.res < 0 {
			err = unix.Errno(-cqe.res)
			n = 0
		}
		if req.Buf != nil && req.Op == poller.OpRead && n > 0 {
			if advErr := req.Buf.AdvanceWriter(n); advErr != nil && err == nil {
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
			N:         n,
			Err:       err,
		}
		out++
		head++
	}
	atomic.StoreUint32(p.cq.head, head)
	return out
}

func (p *Poller) enter(toSubmit uint32, minComplete uint32, flags uint32) error {
	_, _, errno := unix.Syscall6(unix.SYS_IO_URING_ENTER, uintptr(p.fd), uintptr(toSubmit), uintptr(minComplete), uintptr(flags), 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

func (p *Poller) nextUserData() uint64 {
	p.nextID++
	return p.nextID
}

func (p *Poller) sqe(index uint32) *sqe {
	size := unsafe.Sizeof(sqe{})
	return (*sqe)(unsafe.Pointer(&p.sq.sqes[uintptr(index)*size]))
}

func (p *Poller) cqe(index uint32) *cqe {
	size := unsafe.Sizeof(cqe{})
	return (*cqe)(unsafe.Pointer(uintptr(unsafe.Pointer(p.cq.cqes)) + uintptr(index)*size))
}

func uint32Ptr(mem []byte, off uint32) *uint32 {
	return (*uint32)(unsafe.Pointer(&mem[off]))
}

func unmapIfPresent(mem []byte) error {
	if len(mem) == 0 {
		return nil
	}
	return unix.Munmap(mem)
}

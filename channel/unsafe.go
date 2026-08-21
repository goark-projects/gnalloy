package channel

import (
	"sync/atomic"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

const defaultReadBufferSize = 4096

const (
	defaultWriteHighWatermark = 64 * 1024
	defaultWriteLowWatermark  = 32 * 1024
)

// FDReadWriter 抽象 readiness 模型下的非阻塞 fd 读写。
// again 表示底层返回 EAGAIN/EWOULDBLOCK，调用方必须停止本轮读写。
type FDReadWriter interface {
	Read(fd transport.FDRef, dst []byte) (n int, again bool, err error)
	Write(fd transport.FDRef, src []byte) (n int, again bool, err error)
	Close(fd transport.FDRef) error
}

type UnsafeConfig struct {
	ID                 transport.ChannelID
	FD                 transport.FDRef
	Allocator          buffer.Allocator
	Poller             transport.Poller
	ReadWriter         FDReadWriter
	CloseHook          func(*Unsafe)
	ReadBufferSize     int
	WriteHighWatermark int
	WriteLowWatermark  int
}

// Unsafe 是底层 I/O 事件与业务 Pipeline 的分界线。
type Unsafe struct {
	ch             *LocalChannel
	fd             transport.FDRef
	poller         transport.Poller
	rw             FDReadWriter
	closeHook      func(*Unsafe)
	readBufferSize int
	registered     bool
	closed         bool
	inactiveFired  bool

	outHead       *outboundEntry
	outTail       *outboundEntry
	outFree       *outboundEntry
	outboundBytes int64
	writePending  bool
	writeInterest bool

	writeHighWatermark int64
	writeLowWatermark  int64
	writable           atomic.Bool
}

type outboundEntry struct {
	buf  buffer.ByteBuf
	next *outboundEntry
}

func NewUnsafeChannel(cfg UnsafeConfig) (*LocalChannel, *Unsafe) {
	readBufferSize := cfg.ReadBufferSize
	if readBufferSize <= 0 {
		readBufferSize = defaultReadBufferSize
	}
	high := cfg.WriteHighWatermark
	if high <= 0 {
		high = defaultWriteHighWatermark
	}
	low := cfg.WriteLowWatermark
	if low < 0 || low >= high {
		low = high / 2
	}
	u := &Unsafe{
		fd:                 cfg.FD,
		poller:             cfg.Poller,
		rw:                 cfg.ReadWriter,
		closeHook:          cfg.CloseHook,
		readBufferSize:     readBufferSize,
		writeHighWatermark: int64(high),
		writeLowWatermark:  int64(low),
	}
	u.writable.Store(true)
	u.ch = NewLocalChannel(cfg.ID, cfg.Allocator, u)
	return u.ch, u
}

func (u *Unsafe) ID() transport.ChannelID {
	return u.ch.ID()
}

func (u *Unsafe) FD() transport.FDRef {
	return u.fd
}

func (u *Unsafe) Channel() *LocalChannel {
	return u.ch
}

// MarkRegistered 由 EventLoop 在 fd 成功注册到底层 poller 后调用。
func (u *Unsafe) MarkRegistered() {
	u.registered = true
}

// MarkDeregistered 由 EventLoop 在 fd 从底层 poller 移除后调用。
func (u *Unsafe) MarkDeregistered() {
	u.registered = false
}

func (u *Unsafe) HandleEvent(ev transport.PollEvent) {
	if u.closed {
		if ev.Buf != nil {
			ev.Buf.Release()
		}
		if ev.Op == transport.OpClose {
			u.fireInactiveOnce()
		}
		return
	}
	if ev.Model == transport.PollerCompletion {
		u.handleCompletion(ev)
		return
	}
	if ev.Err != nil {
		u.fail(ev.Err)
		return
	}
	u.handleReadiness(ev)
}

func (u *Unsafe) Write(msg any) error {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return ErrInvalidMessage
	}
	if buf.ReadableBytes() == 0 {
		buf.Release()
		return nil
	}
	u.enqueueOutbound(buf)
	return nil
}

func (u *Unsafe) Flush() error {
	return u.flushOutbound()
}

func (u *Unsafe) IsWritable() bool {
	return u.writable.Load()
}

func (u *Unsafe) Close() error {
	if u.closed {
		return nil
	}
	u.closed = true
	u.releaseOutbound()
	if u.registered && u.poller != nil && (u.poller.Backend() == transport.BackendIOUring || u.poller.Backend() == transport.BackendIOCP) {
		if err := u.poller.Submit(transport.IORequest{Op: transport.OpClose, FD: u.fd, ChannelID: u.ID()}); err == nil {
			return nil
		}
	}
	if u.rw != nil {
		err := u.rw.Close(u.fd)
		u.fireInactiveOnce()
		return err
	}
	u.fireInactiveOnce()
	return nil
}

func (u *Unsafe) BeginRead() error {
	if u.poller == nil || u.poller.Model() != transport.PollerCompletion {
		return nil
	}
	return u.submitRead()
}

// Activate 在 Channel 归属 EventLoop 并注册到底层 Poller 后调用。
// readiness 后端等待可读事件，completion 后端需要主动提交首个 read。
func (u *Unsafe) Activate() error {
	u.ch.Pipeline().FireChannelActive()
	if err := u.BeginRead(); err != nil {
		u.fail(err)
		return err
	}
	return nil
}

func (u *Unsafe) handleReadiness(ev transport.PollEvent) {
	if ev.Ready&(transport.ReadyError|transport.ReadyHangup) != 0 {
		_ = u.Close()
		return
	}
	if ev.Ready&transport.ReadyRead != 0 {
		u.readReady()
	}
	if ev.Ready&transport.ReadyWrite != 0 {
		if err := u.flushOutbound(); err != nil {
			u.fail(err)
		}
	}
}

func (u *Unsafe) handleCompletion(ev transport.PollEvent) {
	switch ev.Op {
	case transport.OpRead:
		if ev.Err != nil {
			if ev.Buf != nil {
				ev.Buf.Release()
			}
			u.fail(ev.Err)
			return
		}
		if ev.N > 0 && ev.Buf != nil {
			u.ch.Pipeline().FireChannelRead(ev.Buf)
		} else if ev.Buf != nil {
			ev.Buf.Release()
		}
		if ev.N == 0 {
			_ = u.Close()
			return
		}
		if !u.closed {
			if err := u.submitRead(); err != nil {
				u.fail(err)
			}
		}
	case transport.OpWrite:
		if ev.Buf != nil {
			ev.Buf.Release()
		}
		u.writePending = false
		if ev.Err != nil {
			u.fail(ev.Err)
			return
		}
		u.completeWrite(ev.N)
		if !u.closed {
			if err := u.flushOutbound(); err != nil {
				u.fail(err)
			}
		}
	case transport.OpClose:
		u.finishClose()
	}
}

func (u *Unsafe) readReady() {
	if u.rw == nil {
		return
	}
	for !u.closed {
		buf, err := u.ch.Allocator().Acquire(u.readBufferSize)
		if err != nil {
			u.fail(err)
			return
		}
		view := buf.WritableBytesView()
		n, again, err := u.rw.Read(u.fd, view)
		if n > 0 {
			if advErr := buf.AdvanceWriter(n); advErr != nil {
				buf.Release()
				u.fail(advErr)
				return
			}
			u.ch.Pipeline().FireChannelRead(buf)
		} else {
			buf.Release()
		}
		if err != nil {
			u.fail(err)
			return
		}
		if n == 0 && !again {
			_ = u.Close()
			return
		}
		if again {
			return
		}
	}
}

func (u *Unsafe) flushOutbound() error {
	if u.poller != nil && u.poller.Model() == transport.PollerCompletion {
		return u.submitWrite()
	}
	return u.flushReady()
}

func (u *Unsafe) flushReady() error {
	if u.rw == nil {
		return ErrNoOutboundSink
	}
	for u.outHead != nil {
		buf := u.outHead.buf
		n, again, err := u.rw.Write(u.fd, buf.Bytes())
		if n > 0 {
			if skipErr := buf.SkipBytes(n); skipErr != nil {
				return skipErr
			}
			u.outboundBytes -= int64(n)
			u.updateWritability()
		}
		if err != nil {
			return err
		}
		if buf.ReadableBytes() == 0 {
			u.dequeueOutbound()
			continue
		}
		if again || n == 0 {
			return u.enableWriteInterest()
		}
	}
	return u.disableWriteInterest()
}

func (u *Unsafe) submitWrite() error {
	if u.writePending || u.outHead == nil {
		return nil
	}
	u.writePending = true
	err := u.poller.Submit(transport.IORequest{
		Op:        transport.OpWrite,
		FD:        u.fd,
		ChannelID: u.ID(),
		Buf:       u.outHead.buf,
	})
	if err != nil {
		u.writePending = false
	}
	return err
}

func (u *Unsafe) completeWrite(n int) {
	if u.outHead == nil {
		return
	}
	if n > 0 {
		buf := u.outHead.buf
		if n > buf.ReadableBytes() {
			n = buf.ReadableBytes()
		}
		_ = buf.SkipBytes(n)
		u.outboundBytes -= int64(n)
	}
	if u.outHead.buf.ReadableBytes() == 0 {
		u.dequeueOutbound()
	}
	u.updateWritability()
}

func (u *Unsafe) enqueueOutbound(buf buffer.ByteBuf) {
	e := u.acquireOutboundEntry(buf)
	if u.outTail == nil {
		u.outHead = e
		u.outTail = e
	} else {
		u.outTail.next = e
		u.outTail = e
	}
	u.outboundBytes += int64(buf.ReadableBytes())
	u.updateWritability()
}

func (u *Unsafe) dequeueOutbound() {
	e := u.outHead
	if e == nil {
		return
	}
	u.outHead = e.next
	if u.outHead == nil {
		u.outTail = nil
	}
	e.buf.Release()
	u.releaseOutboundEntry(e)
	u.updateWritability()
}

func (u *Unsafe) acquireOutboundEntry(buf buffer.ByteBuf) *outboundEntry {
	if u.outFree == nil {
		return &outboundEntry{buf: buf}
	}
	e := u.outFree
	u.outFree = e.next
	e.buf = buf
	e.next = nil
	return e
}

func (u *Unsafe) releaseOutboundEntry(e *outboundEntry) {
	e.buf = nil
	e.next = u.outFree
	u.outFree = e
}

func (u *Unsafe) updateWritability() {
	if u.writable.Load() && u.outboundBytes >= u.writeHighWatermark {
		u.writable.Store(false)
		u.ch.Pipeline().FireChannelWritabilityChanged()
		return
	}
	if !u.writable.Load() && u.outboundBytes <= u.writeLowWatermark {
		u.writable.Store(true)
		u.ch.Pipeline().FireChannelWritabilityChanged()
	}
}

func (u *Unsafe) enableWriteInterest() error {
	if u.poller == nil || u.writeInterest {
		return nil
	}
	u.writeInterest = true
	return u.poller.Modify(u.fd, transport.ReadyRead|transport.ReadyWrite)
}

func (u *Unsafe) disableWriteInterest() error {
	if u.poller == nil || !u.writeInterest {
		return nil
	}
	u.writeInterest = false
	return u.poller.Modify(u.fd, transport.ReadyRead)
}

func (u *Unsafe) releaseOutbound() {
	for u.outHead != nil {
		u.dequeueOutbound()
	}
	for u.outFree != nil {
		e := u.outFree
		u.outFree = e.next
		e.next = nil
	}
}

func (u *Unsafe) submitRead() error {
	buf, err := u.ch.Allocator().Acquire(u.readBufferSize)
	if err != nil {
		return err
	}
	err = u.poller.Submit(transport.IORequest{
		Op:        transport.OpRead,
		FD:        u.fd,
		ChannelID: u.ID(),
		Buf:       buf,
	})
	buf.Release()
	return err
}

func (u *Unsafe) fail(err error) {
	u.ch.Pipeline().FireExceptionCaught(err)
	_ = u.Close()
}

func (u *Unsafe) finishClose() {
	if !u.closed {
		u.closed = true
		u.releaseOutbound()
	}
	u.fireInactiveOnce()
}

func (u *Unsafe) fireInactiveOnce() {
	if u.inactiveFired {
		return
	}
	u.inactiveFired = true
	if u.closeHook != nil {
		u.closeHook(u)
	}
	u.ch.Pipeline().FireChannelInactive()
}

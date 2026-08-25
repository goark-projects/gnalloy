package channel

import (
	"sync/atomic"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/timer"
	"goark.dev/gnalloy/transport"
)

const defaultReadBufferSize = 4096

const (
	maxGatheringBuffers = 16
)

// FDReadWriter 抽象 readiness 模型下的非阻塞 fd 读写。
// again 表示底层返回 EAGAIN/EWOULDBLOCK，调用方必须停止本轮读写。
type FDReadWriter interface {
	Read(fd transport.FDRef, dst []byte) (n int, again bool, err error)
	Write(fd transport.FDRef, src []byte) (n int, again bool, err error)
	Close(fd transport.FDRef) error
}

// FDVectorWriter 是支持 gathering write 的 readiness 后端可选能力。
type FDVectorWriter interface {
	Writev(fd transport.FDRef, src [][]byte) (n int, again bool, err error)
}

type UnsafeConfig struct {
	ID                   transport.ChannelID
	FD                   transport.FDRef
	Allocator            buffer.Allocator
	Poller               transport.Poller
	ReadWriter           FDReadWriter
	CloseHook            func(*Unsafe)
	ReadBufferSize       int
	WriteBufferWatermark transport.WriteBufferWatermark
	WriteHighWatermark   int
	WriteLowWatermark    int
	FixedBuffers         bool
	Timer                *timer.Wheel
}

// Unsafe 是底层 I/O 事件与业务 Pipeline 的分界线。
type Unsafe struct {
	ch             *LocalChannel
	fd             transport.FDRef
	poller         transport.Poller
	rw             FDReadWriter
	closeHook      func(*Unsafe)
	readBufferSize int
	fixedBuffers   bool
	registered     bool
	closed         bool
	inactiveFired  bool

	outHead       *outboundEntry
	outTail       *outboundEntry
	outFree       *outboundEntry
	outboundBytes atomic.Int64
	writePending  bool
	writeInterest bool
	writeBatch    []buffer.ByteBuf
	writeSlices   [][]byte
	flushWaiters  []*DefaultPromise
	closePromise  *DefaultPromise

	writeHighWatermark int64
	writeLowWatermark  int64
	writable           atomic.Bool
}

type outboundEntry struct {
	buf     buffer.ByteBuf
	promise Promise
	next    *outboundEntry
}

func NewUnsafeChannel(cfg UnsafeConfig) (*LocalChannel, *Unsafe) {
	readBufferSize := cfg.ReadBufferSize
	if readBufferSize <= 0 {
		readBufferSize = defaultReadBufferSize
	}
	watermark := cfg.WriteBufferWatermark
	if watermark.High == 0 && watermark.Low == 0 && (cfg.WriteHighWatermark != 0 || cfg.WriteLowWatermark != 0) {
		watermark = transport.WriteBufferWatermark{Low: cfg.WriteLowWatermark, High: cfg.WriteHighWatermark}
	}
	watermark = transport.NormalizeWriteBufferWatermark(watermark)
	u := &Unsafe{
		fd:                 cfg.FD,
		poller:             cfg.Poller,
		rw:                 cfg.ReadWriter,
		closeHook:          cfg.CloseHook,
		readBufferSize:     readBufferSize,
		fixedBuffers:       cfg.FixedBuffers,
		writeHighWatermark: int64(watermark.High),
		writeLowWatermark:  int64(watermark.Low),
	}
	u.writable.Store(true)
	u.ch = NewLocalChannelWithTimer(cfg.ID, cfg.Allocator, u, cfg.Timer)
	OptionReadBufferSize.Set(u.ch.Options(), readBufferSize)
	OptionWriteBufferWatermark.Set(u.ch.Options(), watermark)
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
	u.ch.Pipeline().FireChannelRegistered()
}

// MarkDeregistered 由 EventLoop 在 fd 从底层 poller 移除后调用。
func (u *Unsafe) MarkDeregistered() {
	u.registered = false
	u.ch.Pipeline().FireChannelUnregistered()
}

func (u *Unsafe) HandleEvent(ev transport.PollEvent) {
	if u.closed {
		if ev.Buf != nil {
			ev.Buf.Release()
		}
		for _, buf := range ev.Bufs {
			buf.Release()
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
	future := u.WriteFuture(msg)
	if future.IsDone() {
		return future.Err()
	}
	return nil
}

func (u *Unsafe) WriteFuture(msg any) Future {
	if u.closed {
		if buf, ok := msg.(buffer.ByteBuf); ok {
			buf.Release()
		}
		return FailedFuture(ErrPromiseFailed)
	}
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return FailedFuture(ErrInvalidMessage)
	}
	promise := NewPromise()
	if buf.ReadableBytes() == 0 {
		buf.Release()
		promise.SetSuccess()
		return promise
	}
	u.enqueueOutbound(buf, promise)
	return promise
}

func (u *Unsafe) Flush() error {
	future := u.FlushFuture()
	if future.IsDone() {
		return future.Err()
	}
	return nil
}

func (u *Unsafe) FlushFuture() Future {
	promise := NewPromise()
	if u.outHead == nil {
		promise.SetSuccess()
		u.ch.Pipeline().FireFlushComplete()
		return promise
	}
	u.flushWaiters = append(u.flushWaiters, promise)
	if err := u.flushOutbound(); err != nil {
		u.completeFlushWaiters(err)
		return promise
	}
	return promise
}

func (u *Unsafe) IsWritable() bool {
	return u.writable.Load()
}

func (u *Unsafe) PendingOutboundBytes() int64 {
	return u.outboundBytes.Load()
}

func (u *Unsafe) WriteBufferWatermark() transport.WriteBufferWatermark {
	return transport.WriteBufferWatermark{Low: int(u.writeLowWatermark), High: int(u.writeHighWatermark)}
}

func (u *Unsafe) Close() error {
	future := u.CloseFuture()
	if future.IsDone() {
		return future.Err()
	}
	return nil
}

func (u *Unsafe) CloseFuture() Future {
	if u.closed {
		if u.closePromise != nil {
			return u.closePromise
		}
		return SucceededFuture()
	}
	u.closePromise = NewPromise()
	u.closed = true
	u.closeWritability()
	u.releaseOutbound()
	if u.registered && u.poller != nil && (u.poller.Backend() == transport.BackendIOUring || u.poller.Backend() == transport.BackendIOCP) {
		if err := u.poller.Submit(transport.IORequest{Op: transport.OpClose, FD: u.fd, ChannelID: u.ID()}); err == nil {
			return u.closePromise
		}
	}
	if u.rw != nil {
		err := u.rw.Close(u.fd)
		if err != nil {
			u.closePromise.SetFailure(err)
			return u.closePromise
		}
		u.fireInactiveOnce()
		return u.closePromise
	}
	u.fireInactiveOnce()
	return u.closePromise
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
		read := false
		if ev.N > 0 && ev.Buf != nil {
			u.ch.Pipeline().FireChannelRead(ev.Buf)
			read = true
		} else if ev.Buf != nil {
			ev.Buf.Release()
		}
		if read {
			u.ch.Pipeline().FireChannelReadComplete()
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
		for _, buf := range ev.Bufs {
			buf.Release()
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
	read := false
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
			read = true
		} else {
			buf.Release()
		}
		if err != nil {
			u.fail(err)
			return
		}
		if n == 0 && !again {
			if read {
				u.ch.Pipeline().FireChannelReadComplete()
			}
			_ = u.Close()
			return
		}
		if again {
			if read {
				u.ch.Pipeline().FireChannelReadComplete()
			}
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
		n, again, err := u.writeReadyBatch()
		if n > 0 {
			u.completeWrite(n)
		}
		if err != nil {
			return err
		}
		if again || n == 0 {
			return u.enableWriteInterest()
		}
	}
	return u.disableWriteInterest()
}

func (u *Unsafe) writeReadyBatch() (int, bool, error) {
	if vector, ok := u.rw.(FDVectorWriter); ok {
		u.writeSlices = u.writeSlices[:0]
		u.collectOutboundSlices(maxGatheringBuffers)
		if len(u.writeSlices) > 1 {
			return vector.Writev(u.fd, u.writeSlices)
		}
	}
	buf := u.outHead.buf
	return u.rw.Write(u.fd, buf.Bytes())
}

func (u *Unsafe) submitWrite() error {
	if u.writePending || u.outHead == nil {
		return nil
	}
	u.writePending = true
	u.collectOutboundBatch()
	req := transport.IORequest{
		Op:        transport.OpWrite,
		FD:        u.fd,
		ChannelID: u.ID(),
	}
	if len(u.writeBatch) == 1 {
		req.Buf = u.writeBatch[0]
	} else {
		req.Bufs = u.writeBatch
	}
	req = u.prepareIORequest(req)
	err := u.poller.Submit(req)
	if err != nil {
		u.writePending = false
	}
	return err
}

func (u *Unsafe) completeWrite(n int) {
	if u.outHead == nil {
		return
	}
	for n > 0 && u.outHead != nil {
		buf := u.outHead.buf
		readable := buf.ReadableBytes()
		if readable == 0 {
			u.dequeueOutbound()
			continue
		}
		consume := n
		if consume > readable {
			consume = readable
		}
		_ = buf.SkipBytes(consume)
		u.outboundBytes.Add(-int64(consume))
		n -= consume
		if buf.ReadableBytes() == 0 {
			u.dequeueOutbound()
		}
	}
	u.updateWritability()
	if u.outHead == nil {
		u.completeFlushWaiters(nil)
		u.ch.Pipeline().FireFlushComplete()
	}
}

func (u *Unsafe) collectOutboundBatch() {
	u.writeBatch = u.writeBatch[:0]
	limit := maxGatheringBuffers
	if u.fixedBuffers && u.poller != nil && u.poller.Backend() == transport.BackendIOUring {
		limit = 1
	}
	for e := u.outHead; e != nil && len(u.writeBatch) < limit; e = e.next {
		if e.buf.ReadableBytes() == 0 {
			continue
		}
		u.writeBatch = append(u.writeBatch, e.buf)
	}
}

func (u *Unsafe) collectOutboundSlices(limit int) {
	for e := u.outHead; e != nil && len(u.writeSlices) < limit; e = e.next {
		if e.buf.ReadableBytes() == 0 {
			continue
		}
		before := len(u.writeSlices)
		u.writeSlices = e.buf.ReadableSlices(u.writeSlices)
		if len(u.writeSlices) > limit {
			u.writeSlices = u.writeSlices[:limit]
			return
		}
		if len(u.writeSlices) == before {
			return
		}
	}
}

func (u *Unsafe) enqueueOutbound(buf buffer.ByteBuf, promise Promise) {
	e := u.acquireOutboundEntry(buf, promise)
	if u.outTail == nil {
		u.outHead = e
		u.outTail = e
	} else {
		u.outTail.next = e
		u.outTail = e
	}
	u.outboundBytes.Add(int64(buf.ReadableBytes()))
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
	if e.promise != nil {
		e.promise.SetSuccess()
	}
	u.releaseOutboundEntry(e)
	u.updateWritability()
}

func (u *Unsafe) acquireOutboundEntry(buf buffer.ByteBuf, promise Promise) *outboundEntry {
	if u.outFree == nil {
		return &outboundEntry{buf: buf, promise: promise}
	}
	e := u.outFree
	u.outFree = e.next
	e.buf = buf
	e.promise = promise
	e.next = nil
	return e
}

func (u *Unsafe) releaseOutboundEntry(e *outboundEntry) {
	e.buf = nil
	e.promise = nil
	e.next = u.outFree
	u.outFree = e
}

func (u *Unsafe) updateWritability() {
	if u.closed {
		u.closeWritability()
		return
	}
	pending := u.outboundBytes.Load()
	if u.writable.Load() && pending >= u.writeHighWatermark {
		u.writable.Store(false)
		u.ch.Pipeline().FireChannelWritabilityChanged()
		return
	}
	if !u.writable.Load() && pending <= u.writeLowWatermark {
		u.writable.Store(true)
		u.ch.Pipeline().FireChannelWritabilityChanged()
	}
}

func (u *Unsafe) closeWritability() {
	if u.writable.CompareAndSwap(true, false) {
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
		e := u.outHead
		u.outHead = e.next
		if u.outHead == nil {
			u.outTail = nil
		}
		e.buf.Release()
		if e.promise != nil {
			e.promise.SetFailure(ErrPromiseFailed)
		}
		u.releaseOutboundEntry(e)
	}
	u.outboundBytes.Store(0)
	u.completeFlushWaiters(ErrPromiseFailed)
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
	req := u.prepareIORequest(transport.IORequest{
		Op:        transport.OpRead,
		FD:        u.fd,
		ChannelID: u.ID(),
		Buf:       buf,
	})
	err = u.poller.Submit(req)
	buf.Release()
	return err
}

func (u *Unsafe) prepareIORequest(req transport.IORequest) transport.IORequest {
	if !u.fixedBuffers || u.poller == nil || u.poller.Backend() != transport.BackendIOUring || req.Buf == nil {
		return req
	}
	idx, ok := buffer.FixedBufferIndex(req.Buf)
	if !ok {
		return req
	}
	req.UseFixedBuffer = true
	req.FixedBufferIndex = idx
	return req
}

func (u *Unsafe) fail(err error) {
	u.ch.Pipeline().FireExceptionCaught(err)
	_ = u.Close()
}

func (u *Unsafe) completeFlushWaiters(err error) {
	if len(u.flushWaiters) == 0 {
		return
	}
	waiters := u.flushWaiters
	u.flushWaiters = nil
	for _, waiter := range waiters {
		if err != nil {
			waiter.SetFailure(err)
		} else {
			waiter.SetSuccess()
		}
	}
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
	if u.closePromise != nil {
		u.closePromise.SetSuccess()
	}
}

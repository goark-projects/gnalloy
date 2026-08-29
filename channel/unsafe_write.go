package channel

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

func (u *Unsafe) Write(msg any) error {
	if u.closed.Load() {
		if buf, ok := msg.(buffer.ByteBuf); ok {
			buf.Release()
		}
		return ErrPromiseFailed
	}
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return ErrInvalidMessage
	}
	if buf.ReadableBytes() == 0 {
		buf.Release()
		return nil
	}
	u.enqueueOutbound(buf, nil)
	return nil
}

func (u *Unsafe) WriteAndFlush(msg any) error {
	if u.closed.Load() {
		if buf, ok := msg.(buffer.ByteBuf); ok {
			buf.Release()
		}
		return ErrPromiseFailed
	}
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return ErrInvalidMessage
	}
	if buf.ReadableBytes() == 0 {
		buf.Release()
		u.ch.Pipeline().FireFlushComplete()
		return nil
	}
	u.enqueueOutbound(buf, nil)
	return u.flushOutbound()
}

func (u *Unsafe) WriteFuture(msg any) Future {
	if u.closed.Load() {
		if buf, ok := msg.(buffer.ByteBuf); ok {
			buf.Release()
		}
		return FailedFuture(ErrPromiseFailed)
	}
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return FailedFuture(ErrInvalidMessage)
	}
	promise := u.newPromise()
	if buf.ReadableBytes() == 0 {
		buf.Release()
		promise.SetSuccess()
		return promise
	}
	u.enqueueOutbound(buf, promise)
	return promise
}

func (u *Unsafe) Flush() error {
	if u.outHead == nil {
		u.ch.Pipeline().FireFlushComplete()
		return nil
	}
	if err := u.flushOutbound(); err != nil {
		u.completeFlushWaiters(err)
		return err
	}
	return nil
}

func (u *Unsafe) FlushFuture() Future {
	promise := u.newPromise()
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

func (u *Unsafe) flushOutbound() error {
	if u.poller != nil && u.poller.Model() == transport.PollerCompletion {
		if u.readCallback {
			u.deferredFlush = true
			return nil
		}
		return u.submitWrite()
	}
	return u.flushReady()
}

func (u *Unsafe) flushReady() error {
	if u.rw == nil {
		return ErrNoOutboundSink
	}
	spinCount := u.maxWriteSpinCount()
	for spins := 0; u.outHead != nil && spins < spinCount; spins++ {
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
	if u.outHead != nil {
		return u.enableWriteInterest()
	}
	return u.disableWriteInterest()
}

func (u *Unsafe) writeReadyBatch() (int, bool, error) {
	if u.outHead.next == nil {
		buf := u.outHead.buf
		if _, ok := buf.(*buffer.DirectByteBuf); ok {
			return u.rw.Write(u.fd, readableWriteBytes(buf))
		}
	}
	if vector := u.vectorWriter; vector != nil {
		u.writeSlices = u.writeSlices[:0]
		u.collectOutboundSlices(maxGatheringBuffers)
		if len(u.writeSlices) > 1 {
			return vector.Writev(u.fd, u.writeSlices)
		}
	}
	buf := u.outHead.buf
	return u.rw.Write(u.fd, readableWriteBytes(buf))
}

func readableWriteBytes(buf buffer.ByteBuf) []byte {
	if data, ok := buffer.ContiguousReadableBytes(buf); ok {
		return data
	}
	return buf.Bytes()
}

func (u *Unsafe) submitWrite() error {
	req, ok := u.prepareWriteRequest()
	if !ok {
		return nil
	}
	u.writePending = true
	err := u.poller.Submit(req)
	if err != nil {
		u.writePending = false
	}
	return err
}

func (u *Unsafe) prepareWriteRequest() (transport.IORequest, bool) {
	if u.writePending || u.outHead == nil {
		return transport.IORequest{}, false
	}
	u.collectOutboundBatch()
	if len(u.writeBatch) == 0 {
		return transport.IORequest{}, false
	}
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
	return req, true
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

func (u *Unsafe) maxWriteSpinCount() int {
	spinCount := int(u.writeSpinCount.Load())
	if spinCount <= 0 {
		return 1
	}
	return spinCount
}

func (u *Unsafe) newPromise() *DefaultPromise {
	if executor, ok := u.eventExecutor.Load().(FutureListenerExecutor); ok {
		return NewPromiseWithExecutor(executor)
	}
	return NewPromise()
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

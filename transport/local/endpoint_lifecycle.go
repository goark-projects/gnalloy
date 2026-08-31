package local

import (
	"goark.dev/gnalloy/internal/message"
	"goark.dev/gnalloy/timer"
	"goark.dev/gnalloy/transport"
)

func (e *endpoint) Close() error {
	e.close(true)
	return nil
}

func (e *endpoint) close(notifyPeer bool) {
	if e == nil || !e.closed.CompareAndSwap(false, true) {
		return
	}
	e.releaseQueues()
	if e.alloc != nil {
		_ = e.alloc.Close()
	}
	if e.server != nil {
		e.server.removeEndpoint(e)
	}
	if e.writable.Swap(false) {
		e.fireWritabilityChanged()
	}
	e.fireInactive()
	if notifyPeer {
		if peer := e.peer.Load(); peer != nil {
			peer.close(false)
		}
	}
}

func (e *endpoint) releaseQueues() {
	e.mu.Lock()
	outbound := e.outbound
	inbound := e.inbound
	e.outbound = nil
	e.inbound = nil
	e.pending = 0
	e.mu.Unlock()
	message.ReleaseAll(outbound)
	message.ReleaseAll(inbound)
}

func (e *endpoint) fireWritabilityChanged() {
	if e != nil && e.ch != nil {
		e.ch.Pipeline().FireChannelWritabilityChanged()
	}
}

func (e *endpoint) fireFlushComplete() {
	if e != nil && e.ch != nil {
		e.ch.Pipeline().FireFlushComplete()
	}
}

func (e *endpoint) fireInactive() {
	if e == nil || e.ch == nil || !e.inactiveFired.CompareAndSwap(false, true) {
		return
	}
	task := func() {
		e.ch.Pipeline().FireChannelInactive()
		e.ch.Pipeline().FireChannelUnregistered()
	}
	if e.loop == nil || e.loop.Submit(task) != nil {
		task()
	}
}

func timerOf(loop *transport.EventLoop) *timer.Wheel {
	if loop == nil {
		return nil
	}
	return loop.Timer()
}

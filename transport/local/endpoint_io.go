package local

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/internal/message"
)

func (e *endpoint) Write(msg any) error {
	if msg == nil {
		return nil
	}
	size := messageSize(msg)
	e.mu.Lock()
	if e.closed.Load() {
		e.mu.Unlock()
		releaseMessage(msg)
		return ErrClosed
	}
	e.outbound = append(e.outbound, msg)
	e.pending += int64(size)
	fire := e.updateWritabilityLocked()
	e.mu.Unlock()
	if fire {
		e.fireWritabilityChanged()
	}
	return nil
}

func (e *endpoint) Flush() error {
	if e == nil || e.closed.Load() {
		return ErrClosed
	}
	messages, fire := e.takeOutbound()
	if fire {
		e.fireWritabilityChanged()
	}
	if len(messages) == 0 {
		e.fireFlushComplete()
		return nil
	}
	peer := e.peer.Load()
	if peer == nil {
		message.ReleaseAll(messages)
		return ErrNotConnected
	}
	if err := peer.receive(messages); err != nil {
		return err
	}
	e.fireFlushComplete()
	return nil
}

func (e *endpoint) Read() error {
	if e == nil || e.closed.Load() {
		return ErrClosed
	}
	messages := e.takeInbound()
	if len(messages) == 0 {
		return nil
	}
	e.fireInbound(messages)
	return nil
}

func (e *endpoint) receive(messages []any) error {
	if e == nil || e.closed.Load() {
		message.ReleaseAll(messages)
		return ErrClosed
	}
	if !channel.OptionAutoRead.Get(e.ch.Options()) {
		e.enqueueInbound(messages)
		return nil
	}
	return e.dispatchInbound(messages)
}

func (e *endpoint) dispatchInbound(messages []any) error {
	if e == nil || e.loop == nil {
		message.ReleaseAll(messages)
		return ErrClosed
	}
	err := e.loop.Submit(func() {
		if e.closed.Load() {
			message.ReleaseAll(messages)
			return
		}
		e.fireInbound(messages)
	})
	if err != nil {
		message.ReleaseAll(messages)
	}
	return err
}

func (e *endpoint) fireInbound(messages []any) {
	for _, msg := range messages {
		e.ch.Pipeline().FireChannelRead(msg)
	}
	e.ch.Pipeline().FireChannelReadComplete()
}

func (e *endpoint) enqueueInbound(messages []any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed.Load() {
		message.ReleaseAll(messages)
		return
	}
	e.inbound = append(e.inbound, messages...)
}

func (e *endpoint) takeInbound() []any {
	e.mu.Lock()
	defer e.mu.Unlock()
	messages := e.inbound
	e.inbound = nil
	return messages
}

func (e *endpoint) takeOutbound() ([]any, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	messages := e.outbound
	e.outbound = nil
	e.pending = 0
	return messages, e.updateWritabilityLocked()
}

func (e *endpoint) updateWritabilityLocked() bool {
	if e.closed.Load() {
		return e.writable.Swap(false)
	}
	if e.writable.Load() && e.pending >= e.writeHighWatermark {
		e.writable.Store(false)
		return true
	}
	if !e.writable.Load() && e.pending <= e.writeLowWatermark {
		e.writable.Store(true)
		return true
	}
	return false
}

func messageSize(msg any) int {
	switch v := msg.(type) {
	case nil:
		return 0
	case buffer.ByteBuf:
		return v.ReadableBytes()
	case []byte:
		return len(v)
	case string:
		return len(v)
	case interface{ ReadableBytes() int }:
		return v.ReadableBytes()
	default:
		return 0
	}
}

func releaseMessage(msg any) {
	message.Release(msg)
}

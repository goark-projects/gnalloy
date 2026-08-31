package local

import (
	"context"
	"sync"
	"sync/atomic"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

type localEndpointConfig struct {
	id        transport.ChannelID
	loop      *transport.EventLoop
	allocator buffer.Allocator
	watermark transport.WriteBufferWatermark
}

type endpoint struct {
	id     transport.ChannelID
	loop   *transport.EventLoop
	ch     *channel.LocalChannel
	alloc  buffer.Allocator
	server *Server

	mu       sync.Mutex
	outbound []any
	inbound  []any
	pending  int64

	writeHighWatermark int64
	writeLowWatermark  int64
	writable           atomic.Bool
	closed             atomic.Bool
	inactiveFired      atomic.Bool
	peer               atomic.Pointer[endpoint]
}

func newEndpoint(cfg localEndpointConfig) *endpoint {
	watermark := transport.NormalizeWriteBufferWatermark(cfg.watermark)
	ep := &endpoint{
		id:                 cfg.id,
		loop:               cfg.loop,
		alloc:              cfg.allocator,
		writeHighWatermark: int64(watermark.High),
		writeLowWatermark:  int64(watermark.Low),
	}
	ep.writable.Store(true)
	ep.ch = channel.NewLocalChannelWithTimer(cfg.id, cfg.allocator, ep, timerOf(cfg.loop))
	ep.ch.BindEventExecutor(cfg.loop)
	channel.OptionWriteBufferWatermark.Set(ep.ch.Options(), watermark)
	return ep
}

func (e *endpoint) Channel() channel.Channel {
	if e == nil {
		return nil
	}
	return e.ch
}

func (e *endpoint) connect(peer *endpoint) {
	if e != nil {
		e.peer.Store(peer)
	}
}

func (e *endpoint) activate(ctx context.Context) error {
	if e == nil || e.loop == nil {
		return nil
	}
	return e.loop.Invoke(ctx, func() error {
		e.ch.Pipeline().FireChannelRegistered()
		e.ch.Pipeline().FireChannelActive()
		return nil
	})
}

func (e *endpoint) applyOptions() {
	if e == nil || e.ch == nil {
		return
	}
	watermark := transport.NormalizeWriteBufferWatermark(channel.OptionWriteBufferWatermark.Get(e.ch.Options()))
	e.writeHighWatermark = int64(watermark.High)
	e.writeLowWatermark = int64(watermark.Low)
}

func (e *endpoint) IsWritable() bool {
	return e != nil && e.writable.Load()
}

func (e *endpoint) PendingOutboundBytes() int64 {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pending
}

func (e *endpoint) WriteBufferWatermark() transport.WriteBufferWatermark {
	if e == nil {
		return transport.DefaultWriteBufferWatermark()
	}
	return transport.WriteBufferWatermark{Low: int(e.writeLowWatermark), High: int(e.writeHighWatermark)}
}

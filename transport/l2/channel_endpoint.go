package l2

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/internal/message"
	"goark.dev/gnalloy/timer"
	"goark.dev/gnalloy/transport"
)

type channelEndpointOptions struct {
	id     transport.ChannelID
	native Endpoint
	cfg    Config
	timer  *timer.Wheel
}

type channelEndpoint struct {
	id     transport.ChannelID
	native Endpoint
	ch     *channel.LocalChannel
	alloc  buffer.Allocator

	readBufferSize int
	ctx            context.Context
	cancel         context.CancelFunc

	writeMu sync.Mutex
	closed  atomic.Bool

	writeHighWatermark int64
	writeLowWatermark  int64
	writable           atomic.Bool
}

func newChannelEndpoint(opts channelEndpointOptions) (*channelEndpoint, error) {
	if opts.native == nil {
		return nil, ErrInvalidConfig
	}
	alloc := buffer.NewHeapAllocator()
	ctx, cancel := context.WithCancel(context.Background())
	ep := &channelEndpoint{
		id:                 opts.id,
		native:             opts.native,
		alloc:              alloc,
		readBufferSize:     opts.cfg.ReadBufferSize,
		ctx:                ctx,
		cancel:             cancel,
		writeHighWatermark: int64(opts.cfg.WriteBufferWatermark.High),
		writeLowWatermark:  int64(opts.cfg.WriteBufferWatermark.Low),
	}
	ep.writable.Store(true)
	ep.ch = channel.NewLocalChannelWithTimer(opts.id, alloc, ep, opts.timer)
	channel.OptionReadBufferSize.Set(ep.ch.Options(), opts.cfg.ReadBufferSize)
	channel.OptionWriteBufferWatermark.Set(ep.ch.Options(), opts.cfg.WriteBufferWatermark)
	return ep, nil
}

func (e *channelEndpoint) ID() transport.ChannelID {
	if e == nil {
		return 0
	}
	return e.id
}

func (e *channelEndpoint) Addr() string {
	if e == nil || e.native == nil {
		return ""
	}
	return e.native.Addr()
}

func (e *channelEndpoint) Channel() channel.Channel {
	if e == nil {
		return nil
	}
	return e.ch
}

func (e *channelEndpoint) applyOptions() {
	if e == nil || e.ch == nil {
		return
	}
	if readBufferSize := channel.OptionReadBufferSize.Get(e.ch.Options()); readBufferSize > 0 {
		e.readBufferSize = readBufferSize
	}
	watermark := channel.OptionWriteBufferWatermark.Get(e.ch.Options())
	watermark = transport.NormalizeWriteBufferWatermark(watermark)
	e.writeHighWatermark = int64(watermark.High)
	e.writeLowWatermark = int64(watermark.Low)
}

func (e *channelEndpoint) activate() {
	e.ch.Pipeline().FireChannelRegistered()
	e.ch.Pipeline().FireChannelActive()
	if channel.OptionAutoRead.Get(e.ch.Options()) {
		go e.readLoop()
	}
}

func (e *channelEndpoint) Read() error {
	return e.readOnce()
}

func (e *channelEndpoint) Write(msg any) error {
	frame, ok := e.frameFromMessage(msg)
	if !ok || !frame.Valid() {
		releaseMessage(msg)
		return ErrInvalidFrame
	}
	if e.closed.Load() {
		frame.Release()
		return ErrClosed
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	defer frame.Release()
	return e.native.WriteFrame(e.ctx, frame)
}

func (e *channelEndpoint) Flush() error {
	if e.closed.Load() {
		return ErrClosed
	}
	return nil
}

func (e *channelEndpoint) Close() error {
	if e == nil || !e.closed.CompareAndSwap(false, true) {
		return nil
	}
	e.writable.Store(false)
	if e.cancel != nil {
		e.cancel()
	}
	var first error
	if e.native != nil {
		first = e.native.Close()
	}
	e.ch.Pipeline().FireChannelInactive()
	e.ch.Pipeline().FireChannelUnregistered()
	if e.alloc != nil {
		if err := e.alloc.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (e *channelEndpoint) IsWritable() bool {
	return e != nil && e.writable.Load()
}

func (e *channelEndpoint) PendingOutboundBytes() int64 {
	return 0
}

func (e *channelEndpoint) WriteBufferWatermark() transport.WriteBufferWatermark {
	if e == nil {
		return transport.DefaultWriteBufferWatermark()
	}
	return transport.WriteBufferWatermark{Low: int(e.writeLowWatermark), High: int(e.writeHighWatermark)}
}

func (e *channelEndpoint) readLoop() {
	for {
		err := e.readOnce()
		if err == nil {
			continue
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, ErrClosed) {
			return
		}
		e.ch.Pipeline().FireExceptionCaught(err)
		_ = e.Close()
		return
	}
}

func (e *channelEndpoint) readOnce() error {
	if e == nil || e.native == nil {
		return ErrInvalidConfig
	}
	if e.closed.Load() {
		return ErrClosed
	}
	frame, err := e.native.ReadFrame(e.ctx, e.alloc, e.readBufferSize)
	if err != nil {
		return err
	}
	if !frame.Valid() {
		frame.Release()
		return ErrInvalidFrame
	}
	e.ch.Pipeline().FireChannelRead(frame)
	e.ch.Pipeline().FireChannelReadComplete()
	return nil
}

func (e *channelEndpoint) frameFromMessage(msg any) (Frame, bool) {
	switch v := msg.(type) {
	case Frame:
		return v, true
	case *Frame:
		if v == nil {
			return Frame{}, false
		}
		return *v, true
	case buffer.ByteBuf:
		if v == nil {
			return Frame{}, false
		}
		return Frame{Payload: v}, true
	default:
		return Frame{}, false
	}
}

func releaseMessage(msg any) {
	if frame, ok := msg.(Frame); ok {
		frame.Release()
		return
	}
	message.Release(msg)
}

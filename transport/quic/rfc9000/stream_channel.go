package rfc9000

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/timer"
	"goark.dev/gnalloy/transport"
)

const defaultStreamReadBufferSize = 4096

type streamEndpointConfig struct {
	id      transport.ChannelID
	stream  Stream
	timer   *timer.Wheel
	onClose func()
}

type streamEndpoint struct {
	id     transport.ChannelID
	stream Stream
	sink   *streamSink
	ch     *channel.LocalChannel

	readBufferSize int
	readMu         sync.Mutex
	closeOnce      sync.Once
	inactiveOnce   sync.Once
	closed         atomic.Bool
	onClose        func()
}

func newStreamEndpoint(cfg streamEndpointConfig) (*streamEndpoint, error) {
	if cfg.stream == nil {
		return nil, ErrInvalidStream
	}
	alloc := buffer.NewHeapAllocator()
	sink := newStreamSink(cfg.stream)
	endpoint := &streamEndpoint{
		id:             cfg.id,
		stream:         cfg.stream,
		sink:           sink,
		readBufferSize: defaultStreamReadBufferSize,
		onClose:        cfg.onClose,
	}
	endpoint.ch = channel.NewLocalChannelWithTimer(cfg.id, alloc, endpoint, cfg.timer)
	channel.OptionReadBufferSize.Set(endpoint.ch.Options(), defaultStreamReadBufferSize)
	return endpoint, nil
}

func (e *streamEndpoint) ID() transport.ChannelID {
	if e == nil {
		return 0
	}
	return e.id
}

func (e *streamEndpoint) Channel() channel.Channel {
	if e == nil {
		return nil
	}
	return e.ch
}

func (e *streamEndpoint) applyOptions() {
	if e == nil || e.ch == nil {
		return
	}
	readBufferSize := channel.OptionReadBufferSize.Get(e.ch.Options())
	if readBufferSize <= 0 {
		readBufferSize = defaultStreamReadBufferSize
		channel.OptionReadBufferSize.Set(e.ch.Options(), readBufferSize)
	}
	e.readBufferSize = readBufferSize
}

func (e *streamEndpoint) activate(ctx context.Context) {
	if e == nil || e.ch == nil {
		return
	}
	e.ch.Pipeline().FireChannelRegistered()
	e.ch.Pipeline().FireChannelActive()
	if channel.OptionAutoRead.Get(e.ch.Options()) {
		go e.readLoop(ctx)
	}
}

func (e *streamEndpoint) Read() error {
	return e.readOnce(context.Background())
}

func (e *streamEndpoint) Write(msg any) error {
	if e == nil || e.sink == nil {
		releaseStreamMessage(msg)
		return ErrInvalidStream
	}
	return e.sink.Write(msg)
}

func (e *streamEndpoint) Flush() error {
	if e == nil || e.sink == nil {
		return ErrInvalidStream
	}
	return e.sink.Flush()
}

func (e *streamEndpoint) IsWritable() bool {
	return e != nil && e.sink != nil && e.sink.IsWritable()
}

func (e *streamEndpoint) PendingOutboundBytes() int64 {
	return 0
}

func (e *streamEndpoint) WriteBufferWatermark() transport.WriteBufferWatermark {
	return transport.DefaultWriteBufferWatermark()
}

func (e *streamEndpoint) readLoop(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		err := e.readOnce(ctx)
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) || errors.Is(err, ErrClosed) || errors.Is(err, context.Canceled) {
			return
		}
		e.ch.Pipeline().FireExceptionCaught(err)
		_ = e.Close()
		return
	}
}

func (e *streamEndpoint) readOnce(ctx context.Context) error {
	if e == nil || e.ch == nil || e.stream == nil {
		return ErrInvalidStream
	}
	if e.closed.Load() {
		return ErrClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	e.readMu.Lock()
	defer e.readMu.Unlock()
	if e.closed.Load() {
		return ErrClosed
	}
	buf, err := e.ch.Allocator().Acquire(e.readBufferSize)
	if err != nil {
		return err
	}
	n, readErr := e.stream.Read(buf.WritableBytesView())
	if n > 0 {
		if err := buf.AdvanceWriter(n); err != nil {
			buf.Release()
			return err
		}
		e.ch.Pipeline().FireChannelRead(buf)
		e.ch.Pipeline().FireChannelReadComplete()
	} else {
		buf.Release()
	}
	if errors.Is(readErr, io.EOF) {
		e.fireInactive()
	}
	return readErr
}

func (e *streamEndpoint) Close() error {
	if e == nil {
		return nil
	}
	var first error
	e.closeOnce.Do(func() {
		e.closed.Store(true)
		e.stream.CancelRead(0)
		err := e.sink.Close()
		if err != nil && !errors.Is(err, ErrClosed) {
			first = err
		}
		e.fireInactive()
		if e.onClose != nil {
			e.onClose()
		}
		if e.ch != nil && e.ch.Allocator() != nil {
			if err := e.ch.Allocator().Close(); err != nil && first == nil {
				first = err
			}
		}
	})
	return first
}

func (e *streamEndpoint) fireInactive() {
	e.inactiveOnce.Do(func() {
		e.closed.Store(true)
		if e.ch != nil {
			e.ch.Pipeline().FireChannelInactive()
			e.ch.Pipeline().FireChannelUnregistered()
		}
	})
}

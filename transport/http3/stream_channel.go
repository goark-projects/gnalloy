package http3

import (
	"context"
	"errors"
	"io"
	"sync/atomic"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/quic/rfc9000"
)

type streamReader interface {
	io.Reader
	CancelRead(code rfc9000.StreamErrorCode)
}

type streamWriter interface {
	io.Writer
	io.Closer
	CancelWrite(code rfc9000.StreamErrorCode)
}

type streamIdentifier interface {
	ID() rfc9000.StreamID
}

type streamChannelConfig struct {
	ID             transport.ChannelID
	Kind           StreamKind
	Allocator      buffer.Allocator
	ReadBufferSize int
	Reader         streamReader
	Writer         streamWriter
	Initializer    func(channel.Channel) error
	Stats          *sessionStats
	Opened         bool
}

// StreamChannel 是绑定到单条 QUIC stream 的 gnalloy Channel。
type StreamChannel struct {
	kind           StreamKind
	streamID       rfc9000.StreamID
	ch             *channel.LocalChannel
	reader         streamReader
	sink           *streamSink
	readBufferSize int
	closed         atomic.Bool
	inactive       atomic.Bool
	stats          *sessionStats
}

func newStreamChannel(cfg streamChannelConfig) (*StreamChannel, error) {
	if cfg.Reader == nil && cfg.Writer == nil {
		return nil, ErrInvalidStream
	}
	alloc := cfg.Allocator
	if alloc == nil {
		alloc = buffer.NewHeapAllocator()
	}
	readBufferSize := cfg.ReadBufferSize
	if readBufferSize <= 0 {
		readBufferSize = defaultReadBufferSize
	}
	sink := &streamSink{writer: cfg.Writer, stats: cfg.Stats}
	out := &StreamChannel{
		kind:           cfg.Kind,
		streamID:       streamIDOf(cfg.Reader, cfg.Writer),
		reader:         cfg.Reader,
		sink:           sink,
		readBufferSize: readBufferSize,
		stats:          cfg.Stats,
	}
	out.ch = channel.NewLocalChannel(cfg.ID, alloc, sink)
	if cfg.Initializer != nil {
		if err := cfg.Initializer(out.ch); err != nil {
			_ = sink.Close()
			return nil, err
		}
	}
	if cfg.Stats != nil {
		cfg.Stats.recordStream(cfg.Kind, cfg.Opened)
	}
	out.ch.Pipeline().FireChannelRegistered()
	out.ch.Pipeline().FireChannelActive()
	return out, nil
}

// Kind 返回 stream 的 HTTP/3 协议角色。
func (c *StreamChannel) Kind() StreamKind {
	if c == nil {
		return 0
	}
	return c.kind
}

// StreamID 返回底层 QUIC stream 标识。
func (c *StreamChannel) StreamID() rfc9000.StreamID {
	if c == nil {
		return -1
	}
	return c.streamID
}

// Channel 返回业务可见的 gnalloy Channel。
func (c *StreamChannel) Channel() channel.Channel {
	if c == nil {
		return nil
	}
	return c.ch
}

// ReadOnce 从 QUIC stream 读取一次数据并注入 Pipeline。
func (c *StreamChannel) ReadOnce(ctx context.Context) (int, error) {
	if c == nil || c.ch == nil {
		return 0, ErrInvalidStream
	}
	if c.reader == nil {
		return 0, ErrReadUnsupported
	}
	if c.closed.Load() {
		return 0, ErrClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}
	buf, err := c.ch.Allocator().Acquire(c.readBufferSize)
	if err != nil {
		return 0, err
	}
	view := buf.WritableBytesView()
	n, readErr := c.reader.Read(view)
	if n > 0 {
		c.stats.recordReadBytes(n)
		if err := buf.AdvanceWriter(n); err != nil {
			buf.Release()
			return n, err
		}
		c.ch.Pipeline().FireChannelRead(buf)
		c.ch.Pipeline().FireChannelReadComplete()
	} else {
		buf.Release()
	}
	if errors.Is(readErr, io.EOF) {
		c.fireInactiveOnce(false)
	}
	return n, readErr
}

// ReadLoop 持续读取 QUIC stream，直到上下文结束、EOF 或底层错误。
func (c *StreamChannel) ReadLoop(ctx context.Context) error {
	for {
		_, err := c.ReadOnce(ctx)
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

// Close 关闭 stream channel，并向 Pipeline 发布 inactive/unregistered 生命周期事件。
func (c *StreamChannel) Close() error {
	if c == nil {
		return ErrInvalidStream
	}
	if c.closed.Swap(true) {
		return nil
	}
	err := c.ch.Close()
	c.fireInactiveOnce(true)
	return err
}

func (c *StreamChannel) fireInactiveOnce(cancelRead bool) {
	if c == nil || c.ch == nil || !c.inactive.CompareAndSwap(false, true) {
		return
	}
	if cancelRead && c.reader != nil {
		c.reader.CancelRead(0)
	}
	c.closed.Store(true)
	c.stats.recordClosed()
	c.ch.Pipeline().FireChannelInactive()
	c.ch.Pipeline().FireChannelUnregistered()
}

func streamIDOf(reader streamReader, writer streamWriter) rfc9000.StreamID {
	if reader != nil {
		if id, ok := reader.(streamIdentifier); ok {
			return id.ID()
		}
	}
	if writer != nil {
		if id, ok := writer.(streamIdentifier); ok {
			return id.ID()
		}
	}
	return -1
}

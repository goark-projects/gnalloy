package webtransport

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

// StreamKind 描述 WebTransport stream 的方向。
type StreamKind uint8

const (
	// StreamKindBidirectional 表示 WebTransport 双向 stream。
	StreamKindBidirectional StreamKind = iota + 1
	// StreamKindLocalUnidirectional 表示本端发起的 WebTransport 单向发送 stream。
	StreamKindLocalUnidirectional
	// StreamKindRemoteUnidirectional 表示对端发起的 WebTransport 单向接收 stream。
	StreamKindRemoteUnidirectional
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

type streamConfig struct {
	ID             transport.ChannelID
	Kind           StreamKind
	Allocator      buffer.Allocator
	ReadBufferSize int
	Reader         streamReader
	Writer         streamWriter
	SessionID      uint64
}

// Stream 是绑定到单条 WebTransport stream 的 gnalloy Channel。
type Stream struct {
	kind           StreamKind
	streamID       rfc9000.StreamID
	sessionID      uint64
	ch             *channel.LocalChannel
	reader         streamReader
	readBufferSize int
	closed         atomic.Bool
	inactive       atomic.Bool
}

func newStream(cfg streamConfig) (*Stream, error) {
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
	sink := &streamSink{writer: cfg.Writer}
	out := &Stream{
		kind:           cfg.Kind,
		streamID:       streamIDOf(cfg.Reader, cfg.Writer),
		sessionID:      cfg.SessionID,
		reader:         cfg.Reader,
		readBufferSize: readBufferSize,
	}
	out.ch = channel.NewLocalChannel(cfg.ID, alloc, sink)
	out.ch.Pipeline().FireChannelRegistered()
	out.ch.Pipeline().FireChannelActive()
	return out, nil
}

// Kind 返回 WebTransport stream 的方向。
func (s *Stream) Kind() StreamKind {
	if s == nil {
		return 0
	}
	return s.kind
}

// StreamID 返回底层 QUIC stream ID。
func (s *Stream) StreamID() rfc9000.StreamID {
	if s == nil {
		return -1
	}
	return s.streamID
}

// SessionID 返回该 stream 归属的 WebTransport session ID。
func (s *Stream) SessionID() uint64 {
	if s == nil {
		return 0
	}
	return s.sessionID
}

// Channel 返回业务可见的 gnalloy Channel。
func (s *Stream) Channel() channel.Channel {
	if s == nil {
		return nil
	}
	return s.ch
}

// ReadOnce 从 QUIC stream 读取一次 payload 并注入 Pipeline。
func (s *Stream) ReadOnce(ctx context.Context) (int, error) {
	if s == nil || s.ch == nil {
		return 0, ErrInvalidStream
	}
	if s.reader == nil {
		return 0, ErrReadUnsupported
	}
	if s.closed.Load() {
		return 0, ErrClosed
	}
	ctx = normalizeContext(ctx)
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}
	buf, err := s.ch.Allocator().Acquire(s.readBufferSize)
	if err != nil {
		return 0, err
	}
	n, readErr := s.reader.Read(buf.WritableBytesView())
	if n > 0 {
		if err := buf.AdvanceWriter(n); err != nil {
			buf.Release()
			return n, err
		}
		s.ch.Pipeline().FireChannelRead(buf)
		s.ch.Pipeline().FireChannelReadComplete()
	} else {
		buf.Release()
	}
	if errors.Is(readErr, io.EOF) {
		s.fireInactiveOnce(false)
	}
	return n, readErr
}

// ReadLoop 持续读取 WebTransport stream，直到上下文结束、EOF 或底层错误。
func (s *Stream) ReadLoop(ctx context.Context) error {
	for {
		_, err := s.ReadOnce(ctx)
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

// Close 关闭 WebTransport stream channel，并发布 inactive/unregistered 生命周期事件。
func (s *Stream) Close() error {
	if s == nil {
		return ErrInvalidStream
	}
	if s.closed.Swap(true) {
		return nil
	}
	err := s.ch.Close()
	s.fireInactiveOnce(true)
	return err
}

func (s *Stream) fireInactiveOnce(cancelRead bool) {
	if s == nil || s.ch == nil || !s.inactive.CompareAndSwap(false, true) {
		return
	}
	if cancelRead && s.reader != nil {
		s.reader.CancelRead(0)
	}
	s.closed.Store(true)
	s.ch.Pipeline().FireChannelInactive()
	s.ch.Pipeline().FireChannelUnregistered()
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

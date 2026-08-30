package http3

import (
	"context"
	cryptotls "crypto/tls"
	"fmt"
	"sync/atomic"

	"goark.dev/gnalloy/channel"
	codechttp3 "goark.dev/gnalloy/codec/http3"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/quic/rfc9000"
)

// StreamKind 描述 HTTP/3 QUIC stream 的协议角色。
type StreamKind uint8

const (
	StreamKindRequest StreamKind = iota + 1
	StreamKindLocalControl
	StreamKindRemoteControl
	StreamKindLocalQPACKEncoder
	StreamKindLocalQPACKDecoder
	StreamKindRemoteQPACKEncoder
	StreamKindRemoteQPACKDecoder
)

// Session 把 RFC9000 QUIC 连接装配为 HTTP/3 stream channel 工厂。
type Session struct {
	conn rfc9000.Connection
	cfg  Config
	next atomic.Uint64
}

// NewSession 创建 HTTP/3 transport binding。
func NewSession(conn rfc9000.Connection, cfg Config) (*Session, error) {
	if conn == nil {
		return nil, ErrInvalidConnection
	}
	normalized := normalizeConfig(cfg)
	if err := validateConnection(conn, normalized); err != nil {
		return nil, err
	}
	return &Session{conn: conn, cfg: normalized}, nil
}

// Connection 返回底层 RFC9000 QUIC 连接。
func (s *Session) Connection() rfc9000.Connection {
	if s == nil {
		return nil
	}
	return s.conn
}

// OpenRequestStream 打开本端发起的 HTTP/3 request stream。
func (s *Session) OpenRequestStream(ctx context.Context) (*StreamChannel, error) {
	if s == nil || s.conn == nil {
		return nil, ErrInvalidConnection
	}
	stream, err := s.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return s.newStreamChannel(StreamKindRequest, stream, stream, codechttp3.RequestStreamInitializer(s.cfg.Pipeline))
}

// AcceptRequestStream 接受对端发起的 HTTP/3 request stream。
func (s *Session) AcceptRequestStream(ctx context.Context) (*StreamChannel, error) {
	if s == nil || s.conn == nil {
		return nil, ErrInvalidConnection
	}
	stream, err := s.conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return s.newStreamChannel(StreamKindRequest, stream, stream, codechttp3.RequestStreamInitializer(s.cfg.Pipeline))
}

// OpenLocalControlStream 打开本端 HTTP/3 control stream，并在 active 时写出 SETTINGS。
func (s *Session) OpenLocalControlStream(ctx context.Context) (*StreamChannel, error) {
	return s.openSendStream(ctx, StreamKindLocalControl, codechttp3.LocalControlStreamInitializer(s.cfg.Pipeline))
}

// AcceptRemoteControlStream 接受对端 HTTP/3 control stream。
func (s *Session) AcceptRemoteControlStream(ctx context.Context) (*StreamChannel, error) {
	return s.acceptReceiveStream(ctx, StreamKindRemoteControl, codechttp3.RemoteControlStreamInitializer(s.cfg.Pipeline))
}

// OpenQPACKEncoderStream 打开本端 QPACK encoder stream。
func (s *Session) OpenQPACKEncoderStream(ctx context.Context) (*StreamChannel, error) {
	return s.openSendStream(ctx, StreamKindLocalQPACKEncoder, codechttp3.QPACKEncoderStreamInitializer())
}

// OpenQPACKDecoderStream 打开本端 QPACK decoder stream。
func (s *Session) OpenQPACKDecoderStream(ctx context.Context) (*StreamChannel, error) {
	return s.openSendStream(ctx, StreamKindLocalQPACKDecoder, codechttp3.QPACKDecoderStreamInitializer())
}

// AcceptQPACKEncoderStream 接受对端 QPACK encoder stream。
func (s *Session) AcceptQPACKEncoderStream(ctx context.Context) (*StreamChannel, error) {
	return s.acceptReceiveStream(ctx, StreamKindRemoteQPACKEncoder, func(ch channel.Channel) error {
		return codechttp3.ApplyRemoteQPACKEncoderStreamPipeline(ch.Pipeline())
	})
}

// AcceptQPACKDecoderStream 接受对端 QPACK decoder stream。
func (s *Session) AcceptQPACKDecoderStream(ctx context.Context) (*StreamChannel, error) {
	return s.acceptReceiveStream(ctx, StreamKindRemoteQPACKDecoder, func(ch channel.Channel) error {
		return codechttp3.ApplyRemoteQPACKDecoderStreamPipeline(ch.Pipeline())
	})
}

func (s *Session) openSendStream(ctx context.Context, kind StreamKind, init codechttp3.PipelineInitializer) (*StreamChannel, error) {
	if s == nil || s.conn == nil {
		return nil, ErrInvalidConnection
	}
	stream, err := s.conn.OpenUniStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return s.newStreamChannel(kind, nil, stream, init)
}

func (s *Session) acceptReceiveStream(ctx context.Context, kind StreamKind, init codechttp3.PipelineInitializer) (*StreamChannel, error) {
	if s == nil || s.conn == nil {
		return nil, ErrInvalidConnection
	}
	stream, err := s.conn.AcceptUniStream(ctx)
	if err != nil {
		return nil, err
	}
	return s.newStreamChannel(kind, stream, nil, init)
}

func (s *Session) newStreamChannel(kind StreamKind, reader streamReader, writer streamWriter, init codechttp3.PipelineInitializer) (*StreamChannel, error) {
	id := transport.ChannelID(uint64(s.cfg.ChannelIDBase) + s.next.Add(1) - 1)
	return newStreamChannel(streamChannelConfig{
		ID:             id,
		Kind:           kind,
		Allocator:      s.cfg.Allocator,
		ReadBufferSize: s.cfg.ReadBufferSize,
		Reader:         reader,
		Writer:         writer,
		Initializer:    init,
	})
}

func validateConnection(conn rfc9000.Connection, cfg Config) error {
	state := conn.ConnectionState()
	if state.TLS.Version != cryptotls.VersionTLS13 {
		return fmt.Errorf("%w: version %x", ErrInvalidTLSState, state.TLS.Version)
	}
	if !allowedALPN(state.TLS.NegotiatedProtocol, cfg.AllowedALPN) {
		return fmt.Errorf("%w: %s", ErrInvalidALPN, state.TLS.NegotiatedProtocol)
	}
	return nil
}

func allowedALPN(protocol string, allowed []string) bool {
	for _, candidate := range allowed {
		if protocol == candidate {
			return true
		}
	}
	return false
}

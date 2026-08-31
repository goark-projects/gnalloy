package webtransport

import (
	"context"
	"sync/atomic"

	"goark.dev/gnalloy/transport"
	h3transport "goark.dev/gnalloy/transport/http3"
	"goark.dev/gnalloy/transport/quic"
)

// Session 表示一个通过 HTTP/3 extended CONNECT 建立的 WebTransport session。
type Session struct {
	conn            quic.Connection
	connect         *h3transport.StreamChannel
	cfg             Config
	sessionID       uint64
	quarterStreamID uint64
	next            atomic.Uint64
}

// NewSession 把已完成 WebTransport extended CONNECT 的 HTTP/3 request stream 绑定为会话。
func NewSession(conn quic.Connection, connect *h3transport.StreamChannel, cfg Config) (*Session, error) {
	if conn == nil {
		return nil, ErrInvalidConnection
	}
	if connect == nil || connect.Kind() != h3transport.StreamKindRequest {
		return nil, ErrInvalidConnectStream
	}
	quarter, err := sessionIDToQuarterStreamID(connect.StreamID())
	if err != nil {
		return nil, err
	}
	cfg = normalizeConfig(cfg)
	if !cfg.DisableCapabilityValidation {
		if err := validateCapabilities(conn.ConnectionState()); err != nil {
			return nil, err
		}
	}
	return &Session{
		conn:            conn,
		connect:         connect,
		cfg:             cfg,
		sessionID:       uint64(connect.StreamID()),
		quarterStreamID: quarter,
	}, nil
}

// Connection 返回底层 RFC9000 QUIC 连接。
func (s *Session) Connection() quic.Connection {
	if s == nil {
		return nil
	}
	return s.conn
}

// ConnectStream 返回建立该 WebTransport session 的 HTTP/3 CONNECT stream。
func (s *Session) ConnectStream() *h3transport.StreamChannel {
	if s == nil {
		return nil
	}
	return s.connect
}

// SessionID 返回 WebTransport session ID，即 CONNECT stream ID。
func (s *Session) SessionID() uint64 {
	if s == nil {
		return 0
	}
	return s.sessionID
}

// QuarterStreamID 返回 HTTP Datagram 使用的 Quarter Stream ID。
func (s *Session) QuarterStreamID() uint64 {
	if s == nil {
		return 0
	}
	return s.quarterStreamID
}

// OpenBidirectionalStream 打开本端发起的 WebTransport 双向 stream。
func (s *Session) OpenBidirectionalStream(ctx context.Context) (*Stream, error) {
	if s == nil || s.conn == nil {
		return nil, ErrInvalidConnection
	}
	stream, err := s.conn.OpenStreamSync(normalizeContext(ctx))
	if err != nil {
		return nil, err
	}
	if err := writeBidirectionalPrefix(stream, s.sessionID); err != nil {
		stream.CancelWrite(0)
		return nil, err
	}
	return s.newStream(StreamKindBidirectional, stream, stream)
}

// AcceptBidirectionalStream 接受对端发起的 WebTransport 双向 stream。
func (s *Session) AcceptBidirectionalStream(ctx context.Context) (*Stream, error) {
	if s == nil || s.conn == nil {
		return nil, ErrInvalidConnection
	}
	stream, err := s.conn.AcceptStream(normalizeContext(ctx))
	if err != nil {
		return nil, err
	}
	if err := readBidirectionalPrefix(stream, s.sessionID); err != nil {
		stream.CancelRead(0)
		return nil, err
	}
	return s.newStream(StreamKindBidirectional, stream, stream)
}

// OpenUnidirectionalStream 打开本端发起的 WebTransport 单向 stream。
func (s *Session) OpenUnidirectionalStream(ctx context.Context) (*Stream, error) {
	if s == nil || s.conn == nil {
		return nil, ErrInvalidConnection
	}
	stream, err := s.conn.OpenUniStreamSync(normalizeContext(ctx))
	if err != nil {
		return nil, err
	}
	if err := writeUnidirectionalPrefix(stream, s.sessionID); err != nil {
		stream.CancelWrite(0)
		return nil, err
	}
	return s.newStream(StreamKindLocalUnidirectional, nil, stream)
}

// AcceptUnidirectionalStream 接受对端发起的 WebTransport 单向 stream。
func (s *Session) AcceptUnidirectionalStream(ctx context.Context) (*Stream, error) {
	if s == nil || s.conn == nil {
		return nil, ErrInvalidConnection
	}
	stream, err := s.conn.AcceptUniStream(normalizeContext(ctx))
	if err != nil {
		return nil, err
	}
	if err := readUnidirectionalPrefix(stream, s.sessionID); err != nil {
		stream.CancelRead(0)
		return nil, err
	}
	return s.newStream(StreamKindRemoteUnidirectional, stream, nil)
}

// SendDatagram 发送归属当前 WebTransport session 的 HTTP Datagram。
func (s *Session) SendDatagram(payload []byte) error {
	if s == nil || s.conn == nil {
		return ErrInvalidConnection
	}
	if s.cfg.MaxDatagramPayload > 0 && len(payload) > s.cfg.MaxDatagramPayload {
		return ErrDatagramTooLarge
	}
	out, err := encodeDatagram(s.quarterStreamID, payload)
	if err != nil {
		return err
	}
	return s.conn.SendDatagram(out)
}

// ReceiveDatagram 接收归属当前 WebTransport session 的 HTTP Datagram。
func (s *Session) ReceiveDatagram(ctx context.Context) (Datagram, error) {
	if s == nil || s.conn == nil {
		return Datagram{}, ErrInvalidConnection
	}
	for {
		payload, err := s.conn.ReceiveDatagram(normalizeContext(ctx))
		if err != nil {
			return Datagram{}, err
		}
		datagram, err := decodeDatagram(payload)
		if err != nil {
			return Datagram{}, err
		}
		if datagram.QuarterStreamID != s.quarterStreamID {
			continue
		}
		if s.cfg.MaxDatagramPayload > 0 && len(datagram.Payload) > s.cfg.MaxDatagramPayload {
			return Datagram{}, ErrDatagramTooLarge
		}
		datagram.SessionID = s.sessionID
		return datagram, nil
	}
}

func (s *Session) newStream(kind StreamKind, reader streamReader, writer streamWriter) (*Stream, error) {
	id := transport.ChannelID(uint64(s.cfg.ChannelIDBase) + s.next.Add(1) - 1)
	return newStream(streamConfig{
		ID:             id,
		Kind:           kind,
		Allocator:      s.cfg.Allocator,
		ReadBufferSize: s.cfg.ReadBufferSize,
		Reader:         reader,
		Writer:         writer,
		SessionID:      s.sessionID,
	})
}

func validateCapabilities(state quic.State) error {
	if !state.SupportsDatagrams.Local || !state.SupportsDatagrams.Remote {
		return ErrUnsupportedDatagram
	}
	if !state.SupportsStreamResetPartialDelivery.Local || !state.SupportsStreamResetPartialDelivery.Remote {
		return ErrUnsupportedStreamReset
	}
	return nil
}

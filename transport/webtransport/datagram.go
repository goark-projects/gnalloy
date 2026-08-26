package webtransport

import (
	"context"
	"fmt"

	quicwire "goark.dev/gnalloy/transport/quic"
	"goark.dev/gnalloy/transport/quic/rfc9000"
)

// Datagram 是已归属到 WebTransport session 的 HTTP Datagram。
type Datagram struct {
	// SessionID 是建立 WebTransport session 的 CONNECT stream ID。
	SessionID uint64
	// QuarterStreamID 是 HTTP Datagram 载荷中的 Quarter Stream ID。
	QuarterStreamID uint64
	// Payload 是 datagram 业务载荷；该切片归调用方只读使用。
	Payload []byte
}

func encodeDatagram(quarterStreamID uint64, payload []byte) ([]byte, error) {
	out, err := quicwire.AppendVarInt(nil, quarterStreamID)
	if err != nil {
		return nil, err
	}
	out = append(out, payload...)
	return out, nil
}

func decodeDatagram(payload []byte) (Datagram, error) {
	quarter, n, err := quicwire.ParseVarInt(payload)
	if err != nil {
		return Datagram{}, fmt.Errorf("%w: %v", ErrInvalidDatagram, err)
	}
	return Datagram{QuarterStreamID: quarter, Payload: payload[n:]}, nil
}

func sessionIDToQuarterStreamID(streamID rfc9000.StreamID) (uint64, error) {
	if streamID < 0 {
		return 0, ErrInvalidSessionID
	}
	sessionID := uint64(streamID)
	if sessionID&0x03 != 0 {
		return 0, ErrInvalidSessionID
	}
	return sessionID / 4, nil
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

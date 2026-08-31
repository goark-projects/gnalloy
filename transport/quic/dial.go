package quic

import (
	"context"

	nativequic "github.com/quic-go/quic-go"
)

// DialAddr 使用系统 UDP socket 连接远端 RFC9000 QUIC v1 服务端。
func DialAddr(ctx context.Context, addr string, cfg Config) (Connection, error) {
	if addr == "" {
		return nil, ErrMissingAddress
	}
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	conn, err := nativequic.DialAddr(normalizeContext(ctx), addr, normalized.tls, normalized.quic)
	if err != nil {
		return nil, err
	}
	return wrapConnection(conn), nil
}

// DialAddrEarly 使用系统 UDP socket 连接远端 RFC9000 QUIC v1 服务端，并尝试 0-RTT。
func DialAddrEarly(ctx context.Context, addr string, cfg Config) (Connection, error) {
	if addr == "" {
		return nil, ErrMissingAddress
	}
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	if err := validate0RTTClientConfig(normalized.public); err != nil {
		return nil, err
	}
	conn, err := nativequic.DialAddrEarly(normalizeContext(ctx), addr, normalized.tls, normalized.quic)
	if err != nil {
		return nil, err
	}
	return wrapConnection(conn), nil
}

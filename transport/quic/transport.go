package quic

import (
	"context"

	"goark.dev/gnalloy/transport/quic/rfc9000"
)

type (
	// Transport 将 quic-go-backed QUIC stream 接入 Gnalloy Bootstrap/Dialer。
	Transport = rfc9000.Transport
)

// NewTransport 创建 quic-go-backed QUIC stream transport。
func NewTransport(cfg Config) *Transport {
	return rfc9000.NewTransport(cfg)
}

// ListenAddr 在 addr 上创建 RFC9000 QUIC v1 监听器。
func ListenAddr(addr string, cfg Config) (Listener, error) {
	return rfc9000.ListenAddr(addr, cfg)
}

// ListenAddrEarly 在 addr 上创建允许 0-RTT 的 RFC9000 QUIC v1 监听器。
func ListenAddrEarly(addr string, cfg Config) (EarlyListener, error) {
	return rfc9000.ListenAddrEarly(addr, cfg)
}

// DialAddr 使用系统 UDP socket 连接远端 RFC9000 QUIC v1 服务端。
func DialAddr(ctx context.Context, addr string, cfg Config) (Connection, error) {
	return rfc9000.DialAddr(ctx, addr, cfg)
}

// DialAddrEarly 使用系统 UDP socket 连接远端 RFC9000 QUIC v1 服务端，并尝试 0-RTT。
func DialAddrEarly(ctx context.Context, addr string, cfg Config) (Connection, error) {
	return rfc9000.DialAddrEarly(ctx, addr, cfg)
}

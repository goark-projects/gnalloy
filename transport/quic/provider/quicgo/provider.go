package quicgo

import (
	"context"

	quicprovider "goark.dev/gnalloy/transport/quic/provider"
	"goark.dev/gnalloy/transport/quic/rfc9000"
)

const defaultProviderName = "quic-go"

// Provider 是当前默认的 RFC9000 QUIC v1 provider。
type Provider struct {
	name rfc9000.NativeProvider
}

// New 创建 quic-go provider。
func New(name ...string) Provider {
	providerName := rfc9000.NativeProvider(defaultProviderName)
	if len(name) > 0 && name[0] != "" {
		providerName = rfc9000.NativeProvider(name[0])
	}
	return Provider{name: providerName}
}

// Default 返回默认 quic-go provider。
func Default() Provider {
	return New()
}

// NativeSupport 返回当前底层 QUIC provider 的静态能力。
func (p Provider) NativeSupport() rfc9000.NativeSupport {
	support := rfc9000.DetectNativeSupport()
	if p.name != "" {
		support.Provider = p.name
	}
	return support
}

// EvaluateCapabilities 根据角色和配置返回 RFC9000 能力矩阵。
func (p Provider) EvaluateCapabilities(role rfc9000.EndpointRole, cfg rfc9000.Config) (rfc9000.CapabilitySet, error) {
	return rfc9000.EvaluateCapabilities(role, cfg)
}

// ListenAddr 创建常规 RFC9000 QUIC 监听器。
func (p Provider) ListenAddr(addr string, cfg rfc9000.Config) (rfc9000.Listener, error) {
	return rfc9000.ListenAddr(addr, cfg)
}

// ListenAddrEarly 创建允许 0-RTT 的 RFC9000 QUIC 监听器。
func (p Provider) ListenAddrEarly(addr string, cfg rfc9000.Config) (rfc9000.EarlyListener, error) {
	return rfc9000.ListenAddrEarly(addr, cfg)
}

// DialAddr 建立常规 RFC9000 QUIC 连接。
func (p Provider) DialAddr(ctx context.Context, addr string, cfg rfc9000.Config) (rfc9000.Connection, error) {
	return rfc9000.DialAddr(ctx, addr, cfg)
}

// DialAddrEarly 建立 0-RTT RFC9000 QUIC 连接。
func (p Provider) DialAddrEarly(ctx context.Context, addr string, cfg rfc9000.Config) (rfc9000.Connection, error) {
	return rfc9000.DialAddrEarly(ctx, addr, cfg)
}

var _ quicprovider.Provider = Provider{}

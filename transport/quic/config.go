package quic

import "goark.dev/gnalloy/transport/quic/rfc9000"

type (
	// Config 描述 quic-go-backed RFC9000 连接栈的公共配置。
	Config = rfc9000.Config
)

// DefaultConfig 返回 quic-go-backed RFC9000 适配层的安全默认配置。
func DefaultConfig() Config {
	return rfc9000.DefaultConfig()
}

// NormalizeConfig 克隆 TLS 配置并补齐 QUIC v1、TLS 1.3、ALPN 和流控默认值。
func NormalizeConfig(cfg Config) (Config, error) {
	return rfc9000.NormalizeConfig(cfg)
}

// NewClientTokenStore 创建并发安全的 LRU 客户端 token store。
func NewClientTokenStore(maxOrigins int, tokensPerOrigin int) (ClientTokenStore, error) {
	return rfc9000.NewClientTokenStore(maxOrigins, tokensPerOrigin)
}

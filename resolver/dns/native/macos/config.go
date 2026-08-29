package macos

import (
	"context"
	"time"

	resolverdns "goark.dev/gnalloy/resolver/dns"
)

const (
	defaultCommand        = "/usr/sbin/scutil"
	defaultCommandTimeout = 2 * time.Second
)

// ResolverConfig 描述 macOS 系统 DNS 快照 provider 的装配参数。
type ResolverConfig struct {
	// Command 是 scutil 可执行文件路径，空值使用 /usr/sbin/scutil。
	Command string
	// CommandTimeout 限制读取系统 DNS 快照的最长时间，0 表示 2 秒。
	CommandTimeout time.Duration
	// Timeout 是生成的 DNS resolver 单次查询超时。
	Timeout time.Duration
	// DisableTCPFallback 关闭 UDP 截断响应后的 TCP 回退；默认开启。
	DisableTCPFallback bool
}

// Provider 从 macOS 系统 resolver 快照构造 gnalloy DNS 配置。
type Provider struct {
	cfg ResolverConfig
}

// NewProvider 创建 macOS DNS 配置 provider。
func NewProvider(cfg ResolverConfig) Provider {
	return Provider{cfg: cfg}
}

// NewResolver 基于 macOS 系统 DNS 配置创建 gnalloy resolver。
func NewResolver(ctx context.Context, cfg ResolverConfig) (*resolverdns.Resolver, error) {
	dnsConfig, err := NewProvider(cfg).DNSConfig(ctx)
	if err != nil {
		return nil, err
	}
	return resolverdns.NewResolver(dnsConfig), nil
}

func normalizeProviderConfig(cfg ResolverConfig) ResolverConfig {
	if cfg.Command == "" {
		cfg.Command = defaultCommand
	}
	if cfg.CommandTimeout <= 0 {
		cfg.CommandTimeout = defaultCommandTimeout
	}
	return cfg
}

func applyProviderConfig(out resolverdns.Config, cfg ResolverConfig) resolverdns.Config {
	out.Timeout = cfg.Timeout
	out.TCPFallback = !cfg.DisableTCPFallback
	return out
}

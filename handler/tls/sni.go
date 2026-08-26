package tls

import (
	"strings"

	cryptotls "crypto/tls"
)

// ServerNameSelector 根据 ClientHello 中的 SNI 返回服务端 TLS 配置。
type ServerNameSelector func(serverName string) (*cryptotls.Config, error)

// ServerConfigWithSNI 基于 base 配置创建带 SNI 选择能力的服务端配置。
//
// selector 返回 nil 时继续使用 base 配置，便于按域名逐步覆盖证书和 ALPN 策略。
func ServerConfigWithSNI(base *cryptotls.Config, selector ServerNameSelector) *cryptotls.Config {
	cfg := &cryptotls.Config{}
	if base != nil {
		cfg = base.Clone()
	}
	if selector == nil {
		return cfg
	}
	previous := cfg.GetConfigForClient
	cfg.GetConfigForClient = func(hello *cryptotls.ClientHelloInfo) (*cryptotls.Config, error) {
		if selected, err := selector(hello.ServerName); selected != nil || err != nil {
			return selected, err
		}
		if previous != nil {
			return previous(hello)
		}
		return nil, nil
	}
	return cfg
}

// ServerConfigMap 创建大小写不敏感的 SNI 配置映射。
func ServerConfigMap(configs map[string]*cryptotls.Config) ServerNameSelector {
	index := make(map[string]*cryptotls.Config, len(configs))
	for name, cfg := range configs {
		if name == "" || cfg == nil {
			continue
		}
		index[strings.ToLower(name)] = cfg.Clone()
	}
	return func(serverName string) (*cryptotls.Config, error) {
		cfg := index[strings.ToLower(serverName)]
		if cfg == nil {
			return nil, nil
		}
		return cfg.Clone(), nil
	}
}

package standard

import (
	cryptotls "crypto/tls"
	"fmt"
	"net"

	gnalloytls "goark.dev/gnalloy/handler/tls"
)

const defaultProviderName = "crypto/tls"

// Provider 是纯 Go 标准库 TLS provider。
type Provider struct {
	name string
}

// New 创建标准库 TLS provider。
func New(name ...string) Provider {
	providerName := defaultProviderName
	if len(name) > 0 && name[0] != "" {
		providerName = name[0]
	}
	return Provider{name: providerName}
}

// Default 返回默认标准库 TLS provider。
func Default() Provider {
	return New()
}

// Capabilities 返回 crypto/tls 在连接级 handler 场景下的能力。
func (p Provider) Capabilities() gnalloytls.NativeCapabilities {
	name := p.name
	if name == "" {
		name = defaultProviderName
	}
	return gnalloytls.NativeCapabilities{
		Provider: name,
		TLS13:    true,
		ALPN:     true,
		SNI:      true,
	}
}

// Client 创建客户端 TLS 连接。
func (p Provider) Client(conn net.Conn, cfg *cryptotls.Config) (gnalloytls.Conn, error) {
	if conn == nil {
		return nil, fmt.Errorf("%w: nil conn", gnalloytls.ErrInvalidConfig)
	}
	return cryptotls.Client(conn, cloneTLSConfig(cfg)), nil
}

// Server 创建服务端 TLS 连接。
func (p Provider) Server(conn net.Conn, cfg *cryptotls.Config) (gnalloytls.Conn, error) {
	if conn == nil {
		return nil, fmt.Errorf("%w: nil conn", gnalloytls.ErrInvalidConfig)
	}
	return cryptotls.Server(conn, cloneTLSConfig(cfg)), nil
}

func cloneTLSConfig(cfg *cryptotls.Config) *cryptotls.Config {
	if cfg == nil {
		return &cryptotls.Config{}
	}
	clone := cfg.Clone()
	clone.Certificates = append([]cryptotls.Certificate(nil), cfg.Certificates...)
	clone.CipherSuites = append([]uint16(nil), cfg.CipherSuites...)
	clone.CurvePreferences = append([]cryptotls.CurveID(nil), cfg.CurvePreferences...)
	clone.NextProtos = append([]string(nil), cfg.NextProtos...)
	clone.EncryptedClientHelloConfigList = append([]byte(nil), cfg.EncryptedClientHelloConfigList...)
	if cfg.NameToCertificate != nil {
		clone.NameToCertificate = make(map[string]*cryptotls.Certificate, len(cfg.NameToCertificate))
		for name, cert := range cfg.NameToCertificate {
			clone.NameToCertificate[name] = cert
		}
	}
	return clone
}

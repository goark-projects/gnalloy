package tls

import cryptotls "crypto/tls"

// ClientHello 保存服务端握手前可安全观察的客户端 TLS 元数据。
type ClientHello struct {
	ServerName        string
	ALPNProtocols     []string
	CipherSuites      []uint16
	SupportedVersions []uint16
}

// ClientHelloProvider 根据 ClientHello 元数据选择服务端 TLS 配置。
type ClientHelloProvider interface {
	SelectTLSConfig(hello ClientHello) (*cryptotls.Config, error)
}

// ClientHelloProviderFunc 允许用函数实现 ClientHelloProvider。
type ClientHelloProviderFunc func(hello ClientHello) (*cryptotls.Config, error)

// SelectTLSConfig 调用底层函数选择 TLS 配置。
func (f ClientHelloProviderFunc) SelectTLSConfig(hello ClientHello) (*cryptotls.Config, error) {
	if f == nil {
		return nil, nil
	}
	return f(hello)
}

// ServerConfigWithClientHelloProvider 创建带 ClientHello 配置选择能力的服务端配置。
func ServerConfigWithClientHelloProvider(base *cryptotls.Config, provider ClientHelloProvider) *cryptotls.Config {
	cfg := &cryptotls.Config{}
	if base != nil {
		cfg = base.Clone()
	}
	if provider == nil {
		return cfg
	}
	previous := cfg.GetConfigForClient
	cfg.GetConfigForClient = func(info *cryptotls.ClientHelloInfo) (*cryptotls.Config, error) {
		if selected, err := provider.SelectTLSConfig(clientHelloFromInfo(info)); selected != nil || err != nil {
			return cloneTLSConfig(selected), err
		}
		if previous == nil {
			return nil, nil
		}
		selected, err := previous(info)
		return cloneTLSConfig(selected), err
	}
	return cfg
}

func clientHelloFromInfo(info *cryptotls.ClientHelloInfo) ClientHello {
	if info == nil {
		return ClientHello{}
	}
	return ClientHello{
		ServerName:        info.ServerName,
		ALPNProtocols:     append([]string(nil), info.SupportedProtos...),
		CipherSuites:      append([]uint16(nil), info.CipherSuites...),
		SupportedVersions: append([]uint16(nil), info.SupportedVersions...),
	}
}

func cloneTLSConfig(cfg *cryptotls.Config) *cryptotls.Config {
	if cfg == nil {
		return nil
	}
	clone := cfg.Clone()
	clone.Certificates = cloneTLSCertificates(cfg.Certificates)
	clone.NameToCertificate = cloneTLSCertificateMap(cfg.NameToCertificate)
	clone.NextProtos = append([]string(nil), cfg.NextProtos...)
	clone.CipherSuites = append([]uint16(nil), cfg.CipherSuites...)
	clone.CurvePreferences = append([]cryptotls.CurveID(nil), cfg.CurvePreferences...)
	if cfg.RootCAs != nil {
		clone.RootCAs = cfg.RootCAs.Clone()
	}
	if cfg.ClientCAs != nil {
		clone.ClientCAs = cfg.ClientCAs.Clone()
	}
	return clone
}

func cloneTLSCertificates(certs []cryptotls.Certificate) []cryptotls.Certificate {
	if len(certs) == 0 {
		return nil
	}
	out := make([]cryptotls.Certificate, len(certs))
	for i := range certs {
		out[i] = certs[i]
		out[i].Certificate = cloneByteSlices(certs[i].Certificate)
		out[i].OCSPStaple = append([]byte(nil), certs[i].OCSPStaple...)
		out[i].SignedCertificateTimestamps = cloneByteSlices(certs[i].SignedCertificateTimestamps)
	}
	return out
}

func cloneTLSCertificateMap(certs map[string]*cryptotls.Certificate) map[string]*cryptotls.Certificate {
	if len(certs) == 0 {
		return nil
	}
	out := make(map[string]*cryptotls.Certificate, len(certs))
	for name, cert := range certs {
		if cert == nil {
			continue
		}
		clone := cloneTLSCertificates([]cryptotls.Certificate{*cert})[0]
		out[name] = &clone
	}
	return out
}

func cloneByteSlices(values [][]byte) [][]byte {
	if len(values) == 0 {
		return nil
	}
	out := make([][]byte, len(values))
	for i := range values {
		out[i] = append([]byte(nil), values[i]...)
	}
	return out
}

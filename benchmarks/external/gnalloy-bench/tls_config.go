package main

import (
	cryptotls "crypto/tls"
	"strings"

	"goark.dev/gnalloy/benchmarks/external/internal/benchtls"
	handlertls "goark.dev/gnalloy/handler/tls"
)

func serverTLSConfig(cfg config) (*cryptotls.Config, error) {
	certs, err := benchmarkCertificates(cfg)
	if err != nil {
		return nil, err
	}
	version, err := cryptoTLSVersion(cfg.TLSVersion)
	if err != nil {
		return nil, err
	}
	tlsConfig := &cryptotls.Config{
		Certificates: certs,
		MinVersion:   version,
		MaxVersion:   version,
		NextProtos:   alpnProtocols(cfg.ALPN),
	}
	if err := handlertls.ConfigureCipherSuites(tlsConfig, cfg.CipherSuiteIDs); err != nil {
		return nil, err
	}
	return tlsConfig, nil
}

func clientTLSConfig(cfg config) (*cryptotls.Config, error) {
	version, err := cryptoTLSVersion(cfg.TLSVersion)
	if err != nil {
		version = cryptotls.VersionTLS13
	}
	tlsConfig := &cryptotls.Config{
		ServerName:         tlsServerName(),
		InsecureSkipVerify: true,
		MinVersion:         version,
		MaxVersion:         version,
		NextProtos:         alpnProtocols(cfg.ALPN),
	}
	if err := handlertls.ConfigureCipherSuites(tlsConfig, cfg.CipherSuiteIDs); err != nil {
		return nil, err
	}
	return tlsConfig, nil
}

func tlsServerName() string {
	return benchtls.DefaultServerName
}

func alpnProtocols(value string) []string {
	parts := strings.Split(value, ",")
	protocols := make([]string, 0, len(parts))
	for _, part := range parts {
		protocol := strings.TrimSpace(part)
		if protocol != "" {
			protocols = append(protocols, protocol)
		}
	}
	return protocols
}

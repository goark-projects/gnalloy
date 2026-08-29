package main

import (
	cryptotls "crypto/tls"
	"strings"

	"goark.dev/gnalloy/benchmarks/external/internal/benchtls"
)

func serverTLSConfig(cfg config) (*cryptotls.Config, error) {
	cert, err := benchtls.SelfSignedCertificate(tlsServerName())
	if err != nil {
		return nil, err
	}
	return &cryptotls.Config{
		Certificates: []cryptotls.Certificate{cert},
		MinVersion:   cryptotls.VersionTLS13,
		NextProtos:   alpnProtocols(cfg.ALPN),
	}, nil
}

func clientTLSConfig(cfg config) *cryptotls.Config {
	return &cryptotls.Config{
		ServerName:         tlsServerName(),
		InsecureSkipVerify: true,
		MinVersion:         cryptotls.VersionTLS13,
		NextProtos:         alpnProtocols(cfg.ALPN),
	}
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

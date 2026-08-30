package main

import (
	"fmt"
	"strings"

	cryptotls "crypto/tls"
)

const (
	tlsVersion11      = "1.1"
	tlsVersion12      = "1.2"
	tlsVersion13      = "1.3"
	defaultTLSVersion = tlsVersion13
)

func normalizeTLSVersion(value string) (string, error) {
	version := strings.TrimSpace(value)
	if version == "" {
		return defaultTLSVersion, nil
	}
	switch version {
	case tlsVersion11, tlsVersion12, tlsVersion13:
		return version, nil
	default:
		return "", fmt.Errorf("%w: unsupported tls version %s", errInvalidConfig, value)
	}
}

func cryptoTLSVersion(value string) (uint16, error) {
	version, err := normalizeTLSVersion(value)
	if err != nil {
		return 0, err
	}
	switch version {
	case tlsVersion11:
		return cryptotls.VersionTLS11, nil
	case tlsVersion12:
		return cryptotls.VersionTLS12, nil
	case tlsVersion13:
		return cryptotls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("%w: unsupported tls version %s", errInvalidConfig, value)
	}
}

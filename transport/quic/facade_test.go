package quic

import (
	"crypto/tls"
	"errors"
	"testing"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/transport/quic/rfc9000"
)

func TestFacadeExposesQUICGoBackedTransport(t *testing.T) {
	transport := NewTransport(DefaultConfig())
	var serverTransport bootstrap.ServerTransport = transport
	var clientTransport bootstrap.ClientTransport = transport
	_, _ = serverTransport, clientTransport

	support := DetectNativeSupport()
	if support.Provider != NativeProviderQUICGo || !support.RFC9000 || !support.TLS13Only {
		t.Fatalf("support=%+v, want quic-go RFC9000 TLS1.3 provider", support)
	}
	if err := RequireProviderSupported(DefaultProvider()); err != nil {
		t.Fatal(err)
	}
}

func TestFacadeNormalizeConfigUsesRFC9000Semantics(t *testing.T) {
	cfg, err := NormalizeConfig(Config{TLS: &tls.Config{MinVersion: tls.VersionTLS12}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TLS.MinVersion != tls.VersionTLS13 {
		t.Fatalf("min version=%x, want TLS1.3", cfg.TLS.MinVersion)
	}
	if len(cfg.NextProtos) != 1 || cfg.NextProtos[0] != DefaultALPN {
		t.Fatalf("next protos=%v, want default ALPN", cfg.NextProtos)
	}
	if len(cfg.Versions) != 1 || cfg.Versions[0] != Version1 || !cfg.Versions[0].Valid() {
		t.Fatalf("versions=%v, want QUIC v1", cfg.Versions)
	}
}

func TestFacadeErrorsAliasRFC9000(t *testing.T) {
	if !errors.Is(ErrInvalidConfig, rfc9000.ErrInvalidConfig) {
		t.Fatalf("err=%v, want rfc9000 alias", ErrInvalidConfig)
	}
	if Version1.String() != "v1" {
		t.Fatalf("version=%s, want v1", Version1)
	}
}

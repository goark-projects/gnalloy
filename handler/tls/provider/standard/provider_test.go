package standard

import (
	cryptotls "crypto/tls"
	"errors"
	"testing"

	gnalloytls "goark.dev/gnalloy/handler/tls"
)

func TestProviderImplementsTLSProvider(t *testing.T) {
	var _ gnalloytls.Provider = Provider{}
}

func TestProviderCapabilities(t *testing.T) {
	capabilities := Default().Capabilities()
	if capabilities.Provider != "crypto/tls" || !capabilities.TLS13 || !capabilities.ALPN || !capabilities.SNI {
		t.Fatalf("capabilities=%+v", capabilities)
	}
	if capabilities.RequiresCGO || capabilities.QUICPacketProtection {
		t.Fatalf("capabilities=%+v, want pure Go handler provider", capabilities)
	}
}

func TestProviderClonesTLSConfig(t *testing.T) {
	cfg := &cryptotls.Config{
		ServerName: "example.com",
		NextProtos: []string{
			"h2",
			"http/1.1",
		},
	}
	clone := cloneTLSConfig(cfg)
	cfg.ServerName = "mutated.local"
	cfg.NextProtos[0] = "mutated"

	if clone == cfg {
		t.Fatalf("clone must not reuse original config pointer")
	}
	if clone.ServerName != "example.com" || clone.NextProtos[0] != "h2" {
		t.Fatalf("clone=%+v", clone)
	}
}

func TestProviderRejectsNilConn(t *testing.T) {
	_, err := Default().Client(nil, nil)
	if !errors.Is(err, gnalloytls.ErrInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, gnalloytls.ErrInvalidConfig)
	}
	_, err = Default().Server(nil, nil)
	if !errors.Is(err, gnalloytls.ErrInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, gnalloytls.ErrInvalidConfig)
	}
}

func TestNewUsesCustomName(t *testing.T) {
	provider := New("enterprise-crypto")
	if provider.Capabilities().Provider != "enterprise-crypto" {
		t.Fatalf("provider=%q", provider.Capabilities().Provider)
	}
}

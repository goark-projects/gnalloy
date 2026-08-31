package quicgo

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"

	quicprovider "goark.dev/gnalloy/transport/quic/provider"
	"goark.dev/gnalloy/transport/quic/rfc9000"
)

func TestProviderImplementsContract(t *testing.T) {
	var _ quicprovider.Provider = Provider{}
}

func TestProviderReportsNativeSupport(t *testing.T) {
	support := Default().NativeSupport()
	if support.Provider != rfc9000.NativeProviderQUICGo {
		t.Fatalf("provider=%s, want %s", support.Provider, rfc9000.NativeProviderQUICGo)
	}
	if !support.RFC9000 || !support.TLS13Only {
		t.Fatalf("support=%+v, want RFC9000 and TLS 1.3-only boundary", support)
	}
}

func TestProviderAllowsCustomName(t *testing.T) {
	support := New("quic-go-enterprise").NativeSupport()
	if support.Provider != "quic-go-enterprise" {
		t.Fatalf("provider=%s, want custom name", support.Provider)
	}
}

func TestProviderEvaluatesConfigCapabilities(t *testing.T) {
	capabilities, err := Default().EvaluateCapabilities(rfc9000.EndpointRoleClient, rfc9000.Config{
		TLS:                &tls.Config{ClientSessionCache: tls.NewLRUClientSessionCache(8)},
		Enable0RTT:         true,
		EnableWebTransport: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !capabilities.RFC9000 || !capabilities.TLS13 {
		t.Fatalf("base capabilities=%+v", capabilities)
	}
	if !capabilities.ZeroRTT.Enabled || !capabilities.WebTransport.Enabled {
		t.Fatalf("extended capabilities=%+v, want enabled", capabilities)
	}
}

func TestProviderDelegatesBoundaryErrors(t *testing.T) {
	provider := Default()
	if _, err := provider.ListenAddr("", rfc9000.Config{}); !errors.Is(err, rfc9000.ErrMissingAddress) {
		t.Fatalf("listen err=%v, want %v", err, rfc9000.ErrMissingAddress)
	}
	if _, err := provider.DialAddr(context.Background(), "", rfc9000.Config{}); !errors.Is(err, rfc9000.ErrMissingAddress) {
		t.Fatalf("dial err=%v, want %v", err, rfc9000.ErrMissingAddress)
	}
	if _, err := provider.ListenAddr("127.0.0.1:0", rfc9000.Config{}); !errors.Is(err, rfc9000.ErrMissingTLSConfig) {
		t.Fatalf("listen err=%v, want %v", err, rfc9000.ErrMissingTLSConfig)
	}
}

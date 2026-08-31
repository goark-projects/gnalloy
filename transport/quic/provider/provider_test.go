package provider

import (
	"crypto/tls"
	"errors"
	"testing"

	"goark.dev/gnalloy/transport/quic/rfc9000"
)

func TestEvaluateRejectsMissingProvider(t *testing.T) {
	if err := RequireSupported(nil); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("err=%v, want %v", err, ErrUnsupportedProvider)
	}
	var capabilities *capabilityStub
	evaluation := Evaluate(capabilities)
	if evaluation.Supported || len(evaluation.Reasons) == 0 {
		t.Fatalf("evaluation=%+v, want unsupported typed nil", evaluation)
	}
}

func TestEvaluateRejectsInvalidNativeBoundary(t *testing.T) {
	evaluation := Evaluate(&capabilityStub{support: rfc9000.NativeSupport{Provider: "broken"}})
	if evaluation.Supported {
		t.Fatalf("evaluation=%+v, want unsupported", evaluation)
	}
	if len(evaluation.Reasons) != 2 {
		t.Fatalf("reasons=%v, want two boundary failures", evaluation.Reasons)
	}
}

func TestInspectReturnsClientAndServerCapabilities(t *testing.T) {
	capabilities := &capabilityStub{support: validSupport()}
	snapshot, err := Inspect(
		capabilities,
		rfc9000.Config{
			TLS:        &tls.Config{ClientSessionCache: tls.NewLRUClientSessionCache(4)},
			Enable0RTT: true,
		},
		rfc9000.Config{
			TLS:                &tls.Config{},
			EnableWebTransport: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Native.Provider != "test-quic" {
		t.Fatalf("provider=%s, want test-quic", snapshot.Native.Provider)
	}
	if !snapshot.Client.ZeroRTT.Enabled {
		t.Fatalf("client capabilities=%+v, want 0-RTT enabled", snapshot.Client)
	}
	if !snapshot.Server.Datagrams.Enabled || !snapshot.Server.StreamResetPartialDelivery.Enabled {
		t.Fatalf("server capabilities=%+v, want WebTransport prerequisites enabled", snapshot.Server)
	}
}

type capabilityStub struct {
	support rfc9000.NativeSupport
}

func (s *capabilityStub) NativeSupport() rfc9000.NativeSupport {
	if s == nil {
		return rfc9000.NativeSupport{}
	}
	return s.support
}

func (s *capabilityStub) EvaluateCapabilities(role rfc9000.EndpointRole, cfg rfc9000.Config) (rfc9000.CapabilitySet, error) {
	return rfc9000.EvaluateCapabilities(role, cfg)
}

func validSupport() rfc9000.NativeSupport {
	return rfc9000.NativeSupport{
		Provider:  "test-quic",
		RFC9000:   true,
		TLS13Only: true,
		Datagrams: true,
		ZeroRTT:   true,
		QLog:      true,
	}
}

package tls

import (
	"testing"

	"goark.dev/gnalloy/buffer"
)

func TestCopyReadableBytesCopiesCompositeOnce(t *testing.T) {
	first := buffer.NewHeapBuffer(2)
	second := buffer.NewHeapBuffer(2)
	_, _ = first.WriteBytes([]byte("ab"))
	_, _ = second.WriteBytes([]byte("cd"))
	composite := buffer.NewCompositeByteBuf()
	composite.Append(first)
	composite.Append(second)
	defer composite.Release()

	data := copyReadableBytes(composite)
	if string(data) != "abcd" {
		t.Fatalf("data=%q, want abcd", data)
	}
}

func TestEvaluateNativeProviderRequiresTLS13ALPNAndQUIC(t *testing.T) {
	evaluation := EvaluateNativeProvider(nativeProviderStub{
		Provider:             "stub",
		TLS13:                true,
		ALPN:                 true,
		QUICPacketProtection: true,
	})
	if !evaluation.Supported {
		t.Fatalf("evaluation=%+v, want supported", evaluation)
	}

	evaluation = EvaluateNativeProvider(UnsupportedNativeProvider{})
	if evaluation.Supported || len(evaluation.Reasons) == 0 {
		t.Fatalf("evaluation=%+v, want unsupported reasons", evaluation)
	}
}

type nativeProviderStub NativeCapabilities

func (p nativeProviderStub) Capabilities() NativeCapabilities {
	return NativeCapabilities(p)
}

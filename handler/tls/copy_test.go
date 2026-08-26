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

	data := copyReadableBytes(composite, nil)
	if string(data) != "abcd" {
		t.Fatalf("data=%q, want abcd", data)
	}
	releaseBytes(defaultBytePool, data)
}

func TestCopyReadableBytesUsesConfiguredPool(t *testing.T) {
	pool := &trackingBytePool{}
	buf := buffer.NewHeapBuffer(4)
	_, _ = buf.WriteBytes([]byte("pool"))
	defer buf.Release()

	data := copyReadableBytes(buf, pool)
	if string(data) != "pool" {
		t.Fatalf("data=%q, want pool", data)
	}
	if pool.acquired != 1 || pool.released != 0 {
		t.Fatalf("pool acquired=%d released=%d before release", pool.acquired, pool.released)
	}
	releaseBytes(pool, data)
	if pool.released != 1 {
		t.Fatalf("pool released=%d, want 1", pool.released)
	}
}

func TestMemoryConnReleasesInputAfterRead(t *testing.T) {
	pool := &trackingBytePool{}
	conn := newMemoryConn(pool)
	data := copyBytes([]byte("abcd"), pool)
	if err := conn.feedOwned(data); err != nil {
		t.Fatal(err)
	}

	var dst [2]byte
	if n, err := conn.Read(dst[:]); err != nil || n != 2 || string(dst[:]) != "ab" {
		t.Fatalf("first read n=%d err=%v dst=%q", n, err, string(dst[:]))
	}
	if pool.released != 0 {
		t.Fatalf("released=%d before chunk is fully consumed", pool.released)
	}
	if n, err := conn.Read(dst[:]); err != nil || n != 2 || string(dst[:]) != "cd" {
		t.Fatalf("second read n=%d err=%v dst=%q", n, err, string(dst[:]))
	}
	if pool.released != 1 {
		t.Fatalf("released=%d, want input chunk released", pool.released)
	}
}

func TestMemoryConnCloseReleasesQueuedInput(t *testing.T) {
	pool := &trackingBytePool{}
	conn := newMemoryConn(pool)
	data := copyBytes([]byte("cipher"), pool)
	if err := conn.feedOwned(data); err != nil {
		t.Fatal(err)
	}
	if pool.released != 0 {
		t.Fatalf("released=%d before close", pool.released)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if pool.released != 1 {
		t.Fatalf("released=%d, want queued input released", pool.released)
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

type trackingBytePool struct {
	acquired int
	released int
}

func (p *trackingBytePool) Acquire(size int) []byte {
	p.acquired++
	return make([]byte, size)
}

func (p *trackingBytePool) Release([]byte) {
	p.released++
}

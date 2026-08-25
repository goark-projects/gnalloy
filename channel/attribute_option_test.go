package channel

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

func TestAttributeMapTypedAccess(t *testing.T) {
	ch := NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	key := NewAttributeKey[int]("request-count")
	key.Set(ch.Attributes(), 7)

	got, ok := key.Get(ch.Attributes())
	if !ok || got != 7 {
		t.Fatalf("got=%d ok=%v", got, ok)
	}
	removed, ok := key.Remove(ch.Attributes())
	if !ok || removed != 7 {
		t.Fatalf("removed=%d ok=%v", removed, ok)
	}
	if _, ok := key.Get(ch.Attributes()); ok {
		t.Fatal("attribute should be removed")
	}
}

func TestChannelOptionsTypedDefaultsAndSet(t *testing.T) {
	ch := NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if !OptionAutoRead.Get(ch.Options()) {
		t.Fatal("auto read should default to true")
	}
	if got := OptionReadBufferSize.Get(ch.Options()); got != 0 {
		t.Fatalf("read buffer size=%d", got)
	}
	OptionReadBufferSize.Set(ch.Options(), 4096)
	if got := OptionReadBufferSize.Get(ch.Options()); got != 4096 {
		t.Fatalf("read buffer size=%d", got)
	}
	w := transport.WriteBufferWatermark{Low: 1, High: 2}
	OptionWriteBufferWatermark.Set(ch.Options(), w)
	if got := OptionWriteBufferWatermark.Get(ch.Options()); got != w {
		t.Fatalf("watermark=%+v", got)
	}
}

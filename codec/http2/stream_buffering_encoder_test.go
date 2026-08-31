package http2

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel/embedded"
)

func TestStreamBufferingEncoderDrainsWhenActiveStreamCloses(t *testing.T) {
	encoder := NewStreamBufferingEncoder(StreamBufferingEncoderConfig{MaxConcurrentStreams: 1})
	ch, err := embedded.New(encoder)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if _, err := ch.WriteOutbound(HeadersBlock{StreamID: 1, Fields: []HeaderField{{Name: ":method", Value: "GET"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteOutbound(HeadersBlock{StreamID: 3, Fields: []HeaderField{{Name: ":method", Value: "GET"}}}); err != nil {
		t.Fatal(err)
	}
	if got := encoder.PendingStreams(); got != 1 {
		t.Fatalf("pending streams=%d, want 1", got)
	}
	if _, ok := ch.ReadOutbound(); !ok {
		t.Fatal("missing stream 1 headers")
	}
	if msg, ok := ch.ReadOutbound(); ok {
		releaseTestMessage(msg)
		t.Fatalf("unexpected outbound message while stream 3 is buffered: %T", msg)
	}

	body := buffer.NewHeapBuffer(4)
	if _, err := body.WriteBytes([]byte("done")); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteOutbound(DataFrame{StreamID: 1, Flags: FlagEndStream, Data: body}); err != nil {
		t.Fatal(err)
	}
	if got := encoder.PendingStreams(); got != 0 {
		t.Fatalf("pending streams=%d, want 0", got)
	}
	msg, ok := ch.ReadOutbound()
	if !ok {
		t.Fatal("missing outbound DataFrame")
	}
	if _, ok := msg.(DataFrame); !ok {
		releaseTestMessage(msg)
		t.Fatalf("message=%T, want DataFrame", msg)
	}
	releaseTestMessage(msg)
	msg, ok = ch.ReadOutbound()
	if !ok {
		t.Fatal("missing outbound HeadersBlock")
	}
	if _, ok := msg.(HeadersBlock); !ok {
		releaseTestMessage(msg)
		t.Fatalf("message=%T, want HeadersBlock", msg)
	}
	releaseTestMessage(msg)
}

func TestStreamBufferingEncoderDrainsOnSettingsIncrease(t *testing.T) {
	encoder := NewStreamBufferingEncoder(StreamBufferingEncoderConfig{MaxConcurrentStreams: 1})
	ch, err := embedded.New(encoder)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if _, err := ch.WriteOutbound(HeadersBlock{StreamID: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteOutbound(HeadersBlock{StreamID: 3}); err != nil {
		t.Fatal(err)
	}
	ch.ReleaseAll()
	if _, err := ch.WriteInbound(SettingsFrame{Settings: []Setting{{ID: SettingMaxConcurrentStreams, Value: 2}}}); err != nil {
		t.Fatal(err)
	}
	if got := encoder.ActiveStreams(); got != 2 {
		t.Fatalf("active streams=%d, want 2", got)
	}
	if msg, ok := ch.ReadOutbound(); !ok {
		t.Fatal("missing drained stream")
	} else {
		releaseTestMessage(msg)
	}
}

func TestStreamBufferingEncoderRejectsQueueOverflow(t *testing.T) {
	encoder := NewStreamBufferingEncoder(StreamBufferingEncoderConfig{MaxConcurrentStreams: 0, MaxBufferedStreams: 1})
	encoder.SetMaxConcurrentStreams(1)
	ch, err := embedded.New(encoder)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if _, err := ch.WriteOutbound(HeadersBlock{StreamID: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteOutbound(HeadersBlock{StreamID: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteOutbound(HeadersBlock{StreamID: 5}); !errors.Is(err, ErrStreamBufferFull) {
		t.Fatalf("err=%v, want ErrStreamBufferFull", err)
	}
}

func releaseTestMessage(msg any) {
	if releaser, ok := msg.(interface{ Release() }); ok {
		releaser.Release()
	}
}

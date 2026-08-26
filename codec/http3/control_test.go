package http3

import (
	"bytes"
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/channel/embedded"
)

func TestControlStreamRequiresSettingsFirst(t *testing.T) {
	ch, err := embedded.New(NewControlStreamHandler(), http3ExceptionCapture{})
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if _, err := ch.WriteInbound(GoAwayFrame{ID: 0}); err != nil {
		t.Fatal(err)
	}
	msg, ok := ch.ReadInbound()
	if !ok {
		t.Fatal("missing exception")
	}
	if err, ok := msg.(error); !ok || !errors.Is(err, ErrInvalidFrameOrder) {
		t.Fatalf("msg=%v, want ErrInvalidFrameOrder", msg)
	}
}

func TestControlStreamRejectsDataAfterSettings(t *testing.T) {
	ch, err := embedded.New(NewControlStreamHandler(), http3ExceptionCapture{})
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if _, err := ch.WriteInbound(SettingsFrame{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteInbound(DataFrame{Data: testBuf([]byte("bad"))}); err != nil {
		t.Fatal(err)
	}
	if _, ok := ch.ReadInbound(); !ok {
		t.Fatal("settings should pass through")
	}
	msg, ok := ch.ReadInbound()
	if !ok {
		t.Fatal("missing exception")
	}
	if err, ok := msg.(error); !ok || !errors.Is(err, ErrUnsupportedFrame) {
		t.Fatalf("msg=%v, want ErrUnsupportedFrame", msg)
	}
}

func TestStreamTypeDecoderFeedsControlPipeline(t *testing.T) {
	frameDecoder, err := NewDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := embedded.New(NewStreamTypeDecoder(), frameDecoder, NewControlStreamHandler())
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	raw := testBuf([]byte{byte(StreamTypeControl), byte(FrameSettings), 0})
	if _, err := ch.WriteInbound(raw); err != nil {
		t.Fatal(err)
	}
	first, ok := ch.ReadInbound()
	if !ok {
		t.Fatal("missing stream type")
	}
	if st := first.(StreamTypeFrame); st.Type != StreamTypeControl {
		t.Fatalf("stream type=%d, want control", st.Type)
	}
	second, ok := ch.ReadInbound()
	if !ok {
		t.Fatal("missing settings")
	}
	if _, ok := second.(SettingsFrame); !ok {
		t.Fatalf("second=%T, want SettingsFrame", second)
	}
}

func TestStreamTypeEncoderWritesPrefixOnce(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("type", NewStreamTypeEncoder(StreamTypeControl)); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("http3", NewEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	if err := ch.Write(SettingsFrame{}); err != nil {
		t.Fatal(err)
	}
	if err := ch.Write(GoAwayFrame{ID: 0}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 3 {
		t.Fatalf("writes=%d, want stream type plus two frames", len(sink.writes))
	}
	if got := sink.writes[0].(buffer.ByteBuf).Bytes(); !bytes.Equal(got, []byte{byte(StreamTypeControl)}) {
		t.Fatalf("stream type bytes=%v", got)
	}
}

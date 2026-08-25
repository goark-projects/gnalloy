package http3

import (
	"bytes"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestDecoderParsesDataFrameZeroCopy(t *testing.T) {
	decoder, err := NewDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	raw := testBuf([]byte{byte(FrameData), 5, 'h', 'e', 'l', 'l', 'o'})
	in := singleComposite(raw)
	defer in.Release()

	out, err := decoder.Decode(nil, in)
	if err != nil {
		t.Fatal(err)
	}
	frame := out.(DataFrame)
	defer frame.Release()
	if string(frame.Data.Bytes()) != "hello" {
		t.Fatalf("data=%q", frame.Data.Bytes())
	}
	if raw.RefCnt() != 2 {
		t.Fatalf("raw ref=%d, want 2 while data slice is alive", raw.RefCnt())
	}
}

func TestDecoderParsesSettingsAndRejectsDuplicate(t *testing.T) {
	decoder, err := NewDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	in := singleComposite(testBuf([]byte{byte(FrameSettings), 2, 1, 10}))
	defer in.Release()

	out, err := decoder.Decode(nil, in)
	if err != nil {
		t.Fatal(err)
	}
	settings := out.(SettingsFrame)
	if len(settings.Settings) != 1 || settings.Settings[0].ID != 1 || settings.Settings[0].Value != 10 {
		t.Fatalf("settings=%+v", settings)
	}

	dup := singleComposite(testBuf([]byte{byte(FrameSettings), 4, 1, 10, 1, 11}))
	defer dup.Release()
	if _, err := decoder.Decode(nil, dup); err != ErrDuplicateSetting {
		t.Fatalf("err=%v, want %v", err, ErrDuplicateSetting)
	}
}

func TestAppendAndDecodePushPromise(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("http3", NewEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	block := testBuf([]byte("hdr"))
	if err := ch.Write(PushPromiseFrame{PushID: 9, HeaderBlock: block}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	header := sink.writes[0].(buffer.ByteBuf).Bytes()
	if !bytes.Equal(header, []byte{byte(FramePushPromise), 4, 9}) {
		t.Fatalf("header=%v", header)
	}
	if sink.writes[1] != block {
		t.Fatal("header block should pass through without copy")
	}

	decoder, err := NewDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	in := singleComposite(testBuf([]byte{byte(FramePushPromise), 4, 9, 'h', 'd', 'r'}))
	defer in.Release()
	out, err := decoder.Decode(nil, in)
	if err != nil {
		t.Fatal(err)
	}
	frame := out.(PushPromiseFrame)
	defer frame.Release()
	if frame.PushID != 9 || string(frame.HeaderBlock.Bytes()) != "hdr" {
		t.Fatalf("frame=%+v block=%q", frame, frame.HeaderBlock.Bytes())
	}
}

func TestEncoderWritesHeadersFrameWithoutCopyingPayload(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("http3", NewEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	block := testBuf([]byte("qpack"))
	if err := ch.Write(HeadersFrame{HeaderBlock: block}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	if got := sink.writes[0].(buffer.ByteBuf).Bytes(); !bytes.Equal(got, []byte{byte(FrameHeaders), 5}) {
		t.Fatalf("header=%v", got)
	}
	if sink.writes[1] != block {
		t.Fatal("payload should pass through without copy")
	}
}

func BenchmarkDecoderDataFrame(b *testing.B) {
	decoder, err := NewDecoder(1024)
	if err != nil {
		b.Fatal(err)
	}
	payload := []byte{byte(FrameData), 5, 'h', 'e', 'l', 'l', 'o'}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		in := singleComposite(testBuf(payload))
		out, err := decoder.Decode(nil, in)
		if err != nil {
			b.Fatal(err)
		}
		out.(DataFrame).Release()
		in.Release()
	}
}

type captureSink struct {
	writes []any
}

func (s *captureSink) Write(msg any) error {
	s.writes = append(s.writes, msg)
	return nil
}

func (s *captureSink) Flush() error {
	return nil
}

func (s *captureSink) Close() error {
	return nil
}

func (s *captureSink) release() {
	for _, msg := range s.writes {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
}

func testBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

func singleComposite(buf buffer.ByteBuf) *buffer.CompositeByteBuf {
	comp := buffer.NewCompositeByteBuf()
	comp.Append(buf)
	return comp
}

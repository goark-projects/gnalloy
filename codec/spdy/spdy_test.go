package spdy

import (
	"bytes"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestDecoderParsesDataFrameZeroCopy(t *testing.T) {
	decoder, err := NewDecoder(Version3, 1024)
	if err != nil {
		t.Fatal(err)
	}
	raw := testBuf([]byte{0, 0, 0, 1, FlagFIN, 0, 0, 5, 'h', 'e', 'l', 'l', 'o'})
	in := singleComposite(raw)
	defer in.Release()

	out, err := decoder.Decode(nil, in)
	if err != nil {
		t.Fatal(err)
	}
	frame := out.(DataFrame)
	defer frame.Release()
	if frame.StreamID != 1 || !frame.Last() || string(frame.Data.Bytes()) != "hello" {
		t.Fatalf("frame=%+v data=%q", frame, frame.Data.Bytes())
	}
	if raw.RefCnt() != 2 {
		t.Fatalf("raw ref=%d, want 2 while data slice is alive", raw.RefCnt())
	}
}

func TestDecoderParsesFragmentedDataFrameHeader(t *testing.T) {
	decoder, err := NewDecoder(Version3, 1024)
	if err != nil {
		t.Fatal(err)
	}
	wire := []byte{0, 0, 0, 1, FlagFIN, 0, 0, 5, 'h', 'e', 'l', 'l', 'o'}
	in := buffer.NewCompositeByteBuf()
	first := testBuf(wire[:5])
	second := testBuf(wire[5:])
	in.Append(first)
	in.Append(second)
	defer in.Release()

	out, err := decoder.Decode(nil, in)
	if err != nil {
		t.Fatal(err)
	}
	frame := out.(DataFrame)
	defer frame.Release()
	if frame.StreamID != 1 || !frame.Last() || string(frame.Data.Bytes()) != "hello" {
		t.Fatalf("frame=%+v data=%q", frame, frame.Data.Bytes())
	}
}

func TestDecoderParsesFragmentedControlFrameHeader(t *testing.T) {
	decoder, err := NewDecoder(Version3, 1024)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte{0, 0, 0, 3, 0, 0, 0, 1, 5 << 5, 0, 'h', 'd', 'r'}
	header := []byte{0x80, 0x03, 0x00, byte(FrameTypeSynStream), FlagFIN | FlagUnidirectional, 0, 0, byte(len(payload))}
	wire := append(header, payload...)
	in := buffer.NewCompositeByteBuf()
	first := testBuf(wire[:3])
	second := testBuf(wire[3:])
	in.Append(first)
	in.Append(second)
	defer in.Release()

	out, err := decoder.Decode(nil, in)
	if err != nil {
		t.Fatal(err)
	}
	frame := out.(SynStreamFrame)
	defer frame.Release()
	if frame.StreamID != 3 || frame.AssociatedToStreamID != 1 || frame.Priority != 5 || !frame.Last() || !frame.Unidirectional() {
		t.Fatalf("frame=%+v", frame)
	}
	if string(frame.HeaderBlock.Bytes()) != "hdr" {
		t.Fatalf("header block=%q", frame.HeaderBlock.Bytes())
	}
}

func TestDecoderParsesSynStreamFrame(t *testing.T) {
	decoder, err := NewDecoder(Version3, 1024)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte{0, 0, 0, 3, 0, 0, 0, 1, 5 << 5, 0, 'h', 'd', 'r'}
	header := []byte{0x80, 0x03, 0x00, byte(FrameTypeSynStream), FlagFIN | FlagUnidirectional, 0, 0, byte(len(payload))}
	in := singleComposite(testBuf(append(header, payload...)))
	defer in.Release()

	out, err := decoder.Decode(nil, in)
	if err != nil {
		t.Fatal(err)
	}
	frame := out.(SynStreamFrame)
	defer frame.Release()
	if frame.StreamID != 3 || frame.AssociatedToStreamID != 1 || frame.Priority != 5 || !frame.Last() || !frame.Unidirectional() {
		t.Fatalf("frame=%+v", frame)
	}
	if string(frame.HeaderBlock.Bytes()) != "hdr" {
		t.Fatalf("header block=%q", frame.HeaderBlock.Bytes())
	}
}

func TestDecoderRejectsInvalidWindowUpdate(t *testing.T) {
	decoder, err := NewDecoder(Version3, 1024)
	if err != nil {
		t.Fatal(err)
	}
	in := singleComposite(testBuf([]byte{
		0x80, 0x03, 0x00, byte(FrameTypeWindowUpdate), 0, 0, 0, 8,
		0, 0, 0, 1, 0, 0, 0, 0,
	}))
	defer in.Release()

	if _, err := decoder.Decode(nil, in); err != ErrInvalidFrame {
		t.Fatalf("err=%v, want %v", err, ErrInvalidFrame)
	}
}

func TestEncoderWritesDataFrameWithoutCopyingPayload(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("spdy", NewEncoder(Version3)); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	body := testBuf([]byte("hello"))
	if err := ch.Write(DataFrame{StreamID: 1, Flags: FlagFIN, Data: body}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	header := sink.writes[0].(buffer.ByteBuf).Bytes()
	if !bytes.Equal(header, []byte{0, 0, 0, 1, FlagFIN, 0, 0, 5}) {
		t.Fatalf("header=%v", header)
	}
	if sink.writes[1] != body {
		t.Fatal("payload should pass through without copy")
	}
}

func TestEncoderWritesSettingsFrame(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("spdy", NewEncoder(Version3)); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	err := ch.Write(SettingsFrame{
		Flags: FlagSettingsClear,
		Settings: []Setting{{
			ID:    7,
			Value: 65535,
			Flags: FlagPersistValue | FlagPersisted,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := sink.writes[0].(buffer.ByteBuf).Bytes()
	want := []byte{0x80, 0x03, 0x00, byte(FrameTypeSettings), FlagSettingsClear, 0, 0, 12, 0, 0, 0, 1, 0x03, 0, 0, 7, 0, 0, 0xff, 0xff}
	if !bytes.Equal(got, want) {
		t.Fatalf("settings=%v", got)
	}
}

func BenchmarkDecoderDataFrame(b *testing.B) {
	decoder, err := NewDecoder(Version3, 1024)
	if err != nil {
		b.Fatal(err)
	}
	payload := []byte{0, 0, 0, 1, FlagFIN, 0, 0, 5, 'h', 'e', 'l', 'l', 'o'}
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

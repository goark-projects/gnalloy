package memcache

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestFrameDecoderParsesBinaryRequestZeroCopyParts(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	decoder, err := NewFrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("memcache", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	in := testBuf([]byte{
		0x80, 0x01, 0x00, 0x03, 0x08, 0x00, 0x00, 0x02,
		0x00, 0x00, 0x00, 0x10, 0xde, 0xad, 0xbe, 0xef,
		0x00, 0x00, 0x00, 0x00, 0xca, 0xfe, 0xba, 0xbe,
		0, 0, 0, 1, 0, 0, 0x0e, 0x10,
		'k', 'e', 'y',
		'v', 'a', 'l', 'u', 'e',
	})
	ch.Pipeline().FireChannelRead(in)
	if len(collector.msgs) != 1 {
		t.Fatalf("msgs=%d, want 1", len(collector.msgs))
	}
	frame := collector.msgs[0].(Frame)
	defer frame.Release()
	if frame.Magic != MagicRequest || frame.Opcode != OpcodeSet || frame.VBucket != 2 || frame.Opaque != 0xdeadbeef || frame.CAS != 0xcafebabe {
		t.Fatalf("frame=%+v", frame)
	}
	if string(frame.Key.Bytes()) != "key" || string(frame.Value.Bytes()) != "value" || frame.Extras.ReadableBytes() != 8 {
		t.Fatalf("parts extras=%d key=%q value=%q", frame.Extras.ReadableBytes(), frame.Key.Bytes(), frame.Value.Bytes())
	}
	if in.RefCnt() != 3 {
		t.Fatalf("input ref=%d, want 3 while three slices are alive", in.RefCnt())
	}
}

func TestFrameEncoderWritesHeaderAndBodyParts(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("memcache", NewFrameEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	extras := testBuf([]byte{0, 0, 0, 1, 0, 0, 0x0e, 0x10})
	key := testBuf([]byte("key"))
	value := testBuf([]byte("value"))
	frame := NewRequest(OpcodeSet, extras, key, value)
	frame.VBucket = 2
	frame.Opaque = 0x01020304
	if err := ch.Write(frame); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 4 {
		t.Fatalf("writes=%d, want 4", len(sink.writes))
	}
	header := sink.writes[0].(buffer.ByteBuf).Bytes()
	if header[0] != MagicRequest || header[1] != byte(OpcodeSet) || header[2] != 0 || header[3] != 3 || header[4] != 8 {
		t.Fatalf("header=%v", header)
	}
	if got := string(sink.writes[2].(buffer.ByteBuf).Bytes()); got != "key" {
		t.Fatalf("key=%q", got)
	}
	if got := string(sink.writes[3].(buffer.ByteBuf).Bytes()); got != "value" {
		t.Fatalf("value=%q", got)
	}
}

func BenchmarkFrameDecoder(b *testing.B) {
	decoder, err := NewFrameDecoder(1024)
	if err != nil {
		b.Fatal(err)
	}
	payload := []byte{
		0x80, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x03, 0, 0, 0, 1,
		0, 0, 0, 0, 0, 0, 0, 0,
		'k', 'e', 'y',
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		comp := singleComposite(testBuf(payload))
		out, err := decoder.Decode(nil, comp)
		if err != nil {
			b.Fatal(err)
		}
		out.(Frame).Release()
		comp.Release()
	}
}

type captureInbound struct {
	msgs []any
}

func (h *captureInbound) ChannelRead(_ *channel.HandlerContext, msg any) {
	h.msgs = append(h.msgs, msg)
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

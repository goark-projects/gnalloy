package compression

import (
	"bytes"
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestGzipHandlersRoundTrip(t *testing.T) {
	encoder, err := NewGzipEncoder(-1)
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewGzipDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	compressed := encodeWithHandler(t, encoder, []byte("hello gzip"))
	decoded := decodeWithHandler(t, decoder, compressed)
	if string(decoded.Bytes()) != "hello gzip" {
		t.Fatalf("decoded=%q", decoded.Bytes())
	}
	decoded.Release()
}

func TestZlibHandlersRoundTrip(t *testing.T) {
	encoder, err := NewZlibEncoder(-1)
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewZlibDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	compressed := encodeWithHandler(t, encoder, []byte("hello zlib"))
	decoded := decodeWithHandler(t, decoder, compressed)
	if string(decoded.Bytes()) != "hello zlib" {
		t.Fatalf("decoded=%q", decoded.Bytes())
	}
	decoded.Release()
}

func TestDecoderEnforcesMaxDecodedBytes(t *testing.T) {
	encoder, err := NewGzipEncoder(-1)
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewGzipDecoder(4)
	if err != nil {
		t.Fatal(err)
	}
	compressed := encodeWithHandler(t, encoder, []byte("payload"))
	collector := &compressionCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("decoder", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireChannelRead(compressed)
	if !errors.Is(collector.err, ErrDecodedTooLong) {
		t.Fatalf("err=%v, want %v", collector.err, ErrDecodedTooLong)
	}
}

func TestEncoderRejectsInvalidLevel(t *testing.T) {
	_, err := NewGzipEncoder(99)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidConfig)
	}
}

func BenchmarkGzipEncoderComposite(b *testing.B) {
	encoder, err := NewGzipEncoder(-1)
	if err != nil {
		b.Fatal(err)
	}
	benchmarkCompressionEncoder(b, encoder, fragmentedCompressionBuffer("abcdabcd", "efghefgh", "ijklijkl", "mnopmnop"))
}

func BenchmarkZlibEncoderComposite(b *testing.B) {
	encoder, err := NewZlibEncoder(-1)
	if err != nil {
		b.Fatal(err)
	}
	benchmarkCompressionEncoder(b, encoder, fragmentedCompressionBuffer("abcdabcd", "efghefgh", "ijklijkl", "mnopmnop"))
}

func BenchmarkGzipDecoder(b *testing.B) {
	encoder, err := NewGzipEncoder(-1)
	if err != nil {
		b.Fatal(err)
	}
	compressed := encodeWithHandler(b, encoder, []byte("abcdabcd efghefgh ijklijkl mnopmnop"))
	defer compressed.Release()
	decoder, err := NewGzipDecoder(1024)
	if err != nil {
		b.Fatal(err)
	}
	benchmarkCompressionDecoder(b, decoder, compressed)
}

func BenchmarkZlibDecoder(b *testing.B) {
	encoder, err := NewZlibEncoder(-1)
	if err != nil {
		b.Fatal(err)
	}
	compressed := encodeWithHandler(b, encoder, []byte("abcdabcd efghefgh ijklijkl mnopmnop"))
	defer compressed.Release()
	decoder, err := NewZlibDecoder(1024)
	if err != nil {
		b.Fatal(err)
	}
	benchmarkCompressionDecoder(b, decoder, compressed)
}

func benchmarkCompressionEncoder(b *testing.B, encoder *Encoder, payload buffer.ByteBuf) {
	b.Helper()
	b.Cleanup(func() {
		payload.Release()
	})
	sink := &compressionSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("encoder", encoder); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		for _, msg := range sink.writes {
			if buf, ok := msg.(buffer.ByteBuf); ok {
				buf.Release()
			}
		}
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ch.Write(retainReadable(payload)); err != nil {
			b.Fatal(err)
		}
		out := sink.writes[len(sink.writes)-1].(buffer.ByteBuf)
		out.Release()
		sink.writes = sink.writes[:0]
	}
}

func benchmarkCompressionDecoder(b *testing.B, decoder *Decoder, compressed buffer.ByteBuf) {
	b.Helper()
	collector := &compressionCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("decoder", decoder); err != nil {
		b.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch.Pipeline().FireChannelRead(retainReadable(compressed))
		if len(collector.reads) != 1 {
			b.Fatalf("reads=%d", len(collector.reads))
		}
		collector.reads[0].(buffer.ByteBuf).Release()
		collector.reads = collector.reads[:0]
	}
}

func fragmentedCompressionBuffer(parts ...string) buffer.ByteBuf {
	c := buffer.NewCompositeByteBuf()
	for _, part := range parts {
		c.Append(testCompressionBuf([]byte(part)))
	}
	return c
}

func retainReadable(src buffer.ByteBuf) buffer.ByteBuf {
	if src != nil {
		return src.Retain()
	}
	return nil
}

func encodeWithHandler(t testing.TB, encoder *Encoder, payload []byte) buffer.ByteBuf {
	t.Helper()
	sink := &compressionSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("encoder", encoder); err != nil {
		t.Fatal(err)
	}
	in := testCompressionBuf(payload)
	if err := ch.Write(in); err != nil {
		t.Fatal(err)
	}
	if in.RefCnt() != 0 {
		t.Fatalf("input ref=%d, want 0", in.RefCnt())
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	out := sink.writes[0].(buffer.ByteBuf)
	if bytes.Equal(out.Bytes(), payload) {
		t.Fatal("payload was not compressed")
	}
	return out
}

func testCompressionBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

func decodeWithHandler(t *testing.T, decoder *Decoder, compressed buffer.ByteBuf) buffer.ByteBuf {
	t.Helper()
	collector := &compressionCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("decoder", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireChannelRead(compressed)
	if collector.err != nil {
		t.Fatal(collector.err)
	}
	if len(collector.reads) != 1 {
		t.Fatalf("reads=%d, want 1", len(collector.reads))
	}
	return collector.reads[0].(buffer.ByteBuf)
}

type compressionSink struct {
	writes []any
}

func (s *compressionSink) Write(msg any) error {
	s.writes = append(s.writes, msg)
	return nil
}

func (s *compressionSink) Flush() error {
	return nil
}

func (s *compressionSink) Close() error {
	return nil
}

type compressionCollector struct {
	reads []any
	err   error
}

func (c *compressionCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	c.reads = append(c.reads, msg)
}

func (c *compressionCollector) ExceptionCaught(_ *channel.HandlerContext, err error) {
	c.err = err
}

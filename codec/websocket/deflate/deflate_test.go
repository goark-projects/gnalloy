package deflate

import (
	"errors"
	"strings"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/websocket"
)

type frameSink struct {
	frames []websocket.Frame
}

func (s *frameSink) Write(msg any) error {
	if frame, ok := msg.(websocket.Frame); ok {
		s.frames = append(s.frames, frame)
	}
	return nil
}

func (s *frameSink) Flush() error { return nil }
func (s *frameSink) Close() error { return nil }
func (s *frameSink) release() {
	for _, frame := range s.frames {
		frame.Release()
	}
}

type frameCollector struct {
	frames []websocket.Frame
}

func (c *frameCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	if frame, ok := msg.(websocket.Frame); ok {
		c.frames = append(c.frames, frame)
	}
}

func (c *frameCollector) release() {
	for _, frame := range c.frames {
		frame.Release()
	}
}

type errorCollector struct {
	errs []error
}

func (c *errorCollector) ExceptionCaught(_ *channel.HandlerContext, err error) {
	c.errs = append(c.errs, err)
}

func TestOfferAndParse(t *testing.T) {
	offer := Offer(Parameters{ServerNoContextTakeover: true, ClientNoContextTakeover: true, ClientMaxWindowBits: "15"})
	if !strings.Contains(offer, ExtensionName) || !strings.Contains(offer, "client_no_context_takeover") {
		t.Fatalf("offer=%q", offer)
	}
	params, ok, err := Parse("x-test, " + offer)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !params.ServerNoContextTakeover || !params.ClientNoContextTakeover || params.ClientMaxWindowBits != "15" {
		t.Fatalf("params=%+v ok=%v", params, ok)
	}
	if _, _, err := Parse(ExtensionName + "; server_no_context_takeover; server_no_context_takeover"); !errors.Is(err, ErrInvalidExtension) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidExtension)
	}
}

func TestCompressorDecompressorRoundTrip(t *testing.T) {
	compressed := compressOutboundFrame(t, websocket.Frame{
		Final:   true,
		Opcode:  websocket.OpcodeText,
		Payload: testBuf([]byte("hello hello hello")),
	})
	defer compressed.Release()
	if !compressed.RSV1 || compressed.Opcode != websocket.OpcodeText {
		t.Fatalf("compressed=%+v", compressed)
	}
	collector := decompressInboundFrame(t, compressed)
	defer collector.release()
	if len(collector.frames) != 1 {
		t.Fatalf("frames=%d, want 1", len(collector.frames))
	}
	frame := collector.frames[0]
	if frame.RSV1 || frame.Opcode != websocket.OpcodeText || string(frame.Payload.Bytes()) != "hello hello hello" {
		t.Fatalf("frame=%+v payload=%q", frame, frame.Payload.Bytes())
	}
}

func TestCompressorPassesControlFrames(t *testing.T) {
	sink := &frameSink{}
	compressor, err := NewCompressor(Config{})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("deflate", compressor)
	defer sink.release()

	if err := ch.Write(websocket.Frame{Final: true, Opcode: websocket.OpcodePing, Payload: testBuf([]byte("hb"))}); err != nil {
		t.Fatal(err)
	}
	if len(sink.frames) != 1 || sink.frames[0].RSV1 || sink.frames[0].Opcode != websocket.OpcodePing {
		t.Fatalf("frames=%+v", sink.frames)
	}
}

func TestDecompressorRejectsControlRSV(t *testing.T) {
	decompressor, err := NewDecompressor(Config{})
	if err != nil {
		t.Fatal(err)
	}
	errs := &errorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("deflate", decompressor)
	_ = ch.Pipeline().AddLast("errors", errs)

	ch.Pipeline().FireChannelRead(websocket.Frame{Final: true, Opcode: websocket.OpcodePing, RSV1: true, Payload: testBuf([]byte("x"))})
	if len(errs.errs) != 1 || !errors.Is(errs.errs[0], ErrInvalidFrame) {
		t.Fatalf("errs=%v, want ErrInvalidFrame", errs.errs)
	}
}

func TestDecompressorRejectsInflatedLimit(t *testing.T) {
	compressed, err := compressMessage([]byte("123456789"), defaultCompressionLevel(t))
	if err != nil {
		t.Fatal(err)
	}
	decompressor, err := NewDecompressor(Config{MaxMessageBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	errs := &errorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("deflate", decompressor)
	_ = ch.Pipeline().AddLast("errors", errs)

	ch.Pipeline().FireChannelRead(websocket.Frame{Final: true, Opcode: websocket.OpcodeBinary, RSV1: true, Payload: testBuf(compressed)})
	if len(errs.errs) != 1 || !errors.Is(errs.errs[0], codec.ErrFrameTooLong) {
		t.Fatalf("errs=%v, want ErrFrameTooLong", errs.errs)
	}
}

func TestFragmentedMessageCompressesOnFinalContinuation(t *testing.T) {
	sink := &frameSink{}
	compressor, err := NewCompressor(Config{})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("deflate", compressor)
	defer sink.release()

	if err := ch.Write(websocket.Frame{Final: false, Opcode: websocket.OpcodeText, Payload: testBuf([]byte("hel"))}); err != nil {
		t.Fatal(err)
	}
	if len(sink.frames) != 0 {
		t.Fatalf("frames=%d, want 0 before final continuation", len(sink.frames))
	}
	if err := ch.Write(websocket.Frame{Final: true, Opcode: websocket.OpcodeContinuation, Payload: testBuf([]byte("lo"))}); err != nil {
		t.Fatal(err)
	}
	if len(sink.frames) != 1 || !sink.frames[0].Final || sink.frames[0].Opcode != websocket.OpcodeText || !sink.frames[0].RSV1 {
		t.Fatalf("frames=%+v", sink.frames)
	}
	collector := decompressInboundFrame(t, sink.frames[0])
	defer collector.release()
	if len(collector.frames) != 1 || string(collector.frames[0].Payload.Bytes()) != "hello" {
		t.Fatalf("frames=%+v", collector.frames)
	}
}

func BenchmarkCompressorCompositePayload(b *testing.B) {
	sink := &frameSink{}
	compressor, err := NewCompressor(Config{})
	if err != nil {
		b.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("deflate", compressor)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ch.Write(websocket.Frame{
			Final:   true,
			Opcode:  websocket.OpcodeBinary,
			Payload: fragmentedDeflatePayload("abcdabcd", "efghefgh", "ijklijkl", "mnopmnop"),
		}); err != nil {
			b.Fatal(err)
		}
		sink.release()
		sink.frames = sink.frames[:0]
	}
}

func BenchmarkDecompressorPayload(b *testing.B) {
	compressed, err := compressMessage([]byte("abcdabcd efghefgh ijklijkl mnopmnop"), defaultCompressionLevel(b))
	if err != nil {
		b.Fatal(err)
	}
	decompressor, err := NewDecompressor(Config{})
	if err != nil {
		b.Fatal(err)
	}
	collector := &frameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("deflate", decompressor)
	_ = ch.Pipeline().AddLast("collector", collector)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch.Pipeline().FireChannelRead(websocket.Frame{
			Final:   true,
			Opcode:  websocket.OpcodeBinary,
			RSV1:    true,
			Payload: testBuf(compressed),
		})
		if len(collector.frames) != 1 {
			b.Fatalf("frames=%d", len(collector.frames))
		}
		collector.release()
		collector.frames = collector.frames[:0]
	}
}

func BenchmarkCompressMessage(b *testing.B) {
	payload := []byte("abcdabcd efghefgh ijklijkl mnopmnop")
	level := defaultCompressionLevel(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := compressMessage(payload, level)
		if err != nil {
			b.Fatal(err)
		}
		if len(out) == 0 {
			b.Fatal("empty compressed payload")
		}
	}
}

func BenchmarkDecompressMessage(b *testing.B) {
	compressed, err := compressMessage([]byte("abcdabcd efghefgh ijklijkl mnopmnop"), defaultCompressionLevel(b))
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := decompressMessage(compressed, 1024)
		if err != nil {
			b.Fatal(err)
		}
		if len(out) == 0 {
			b.Fatal("empty decompressed payload")
		}
	}
}

func fragmentedDeflatePayload(parts ...string) buffer.ByteBuf {
	c := buffer.NewCompositeByteBuf()
	for _, part := range parts {
		c.Append(testBuf([]byte(part)))
	}
	return c
}

func compressOutboundFrame(t *testing.T, frame websocket.Frame) websocket.Frame {
	t.Helper()
	sink := &frameSink{}
	compressor, err := NewCompressor(Config{})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("deflate", compressor)
	if err := ch.Write(frame); err != nil {
		t.Fatal(err)
	}
	if len(sink.frames) != 1 {
		t.Fatalf("frames=%d, want 1", len(sink.frames))
	}
	return sink.frames[0]
}

func decompressInboundFrame(t *testing.T, frame websocket.Frame) *frameCollector {
	t.Helper()
	if frame.Payload != nil {
		frame.Payload.Retain()
	}
	decompressor, err := NewDecompressor(Config{})
	if err != nil {
		t.Fatal(err)
	}
	collector := &frameCollector{}
	errs := &errorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("deflate", decompressor)
	_ = ch.Pipeline().AddLast("collector", collector)
	_ = ch.Pipeline().AddLast("errors", errs)
	ch.Pipeline().FireChannelRead(frame)
	if len(errs.errs) > 0 {
		t.Fatalf("decompress errors=%v", errs.errs)
	}
	return collector
}

func defaultCompressionLevel(t testing.TB) int {
	t.Helper()
	cfg, err := normalizeConfig(Config{})
	if err != nil {
		t.Fatal(err)
	}
	return cfg.CompressionLevel
}

func testBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

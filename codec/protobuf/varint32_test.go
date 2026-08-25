package protobuf

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

type frameCollector struct {
	frames []buffer.ByteBuf
}

func (c *frameCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		c.frames = append(c.frames, buf)
	}
}

func (c *frameCollector) release() {
	for _, frame := range c.frames {
		frame.Release()
	}
	c.frames = nil
}

func TestVarint32FrameDecoder(t *testing.T) {
	decoder, err := NewVarint32FrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &frameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)
	defer collector.release()

	ch.Pipeline().FireChannelRead(testBuf([]byte{5, 'h'}))
	if len(collector.frames) != 0 {
		t.Fatalf("frames=%d, want 0", len(collector.frames))
	}
	ch.Pipeline().FireChannelRead(testBuf([]byte("ello")))
	if len(collector.frames) != 1 || string(collector.frames[0].Bytes()) != "hello" {
		t.Fatalf("frames=%v", collector.frames)
	}
}

func TestVarint32LengthFieldPrepender(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("prepender", NewVarint32LengthFieldPrepender())
	defer sink.release()

	if err := ch.Write(testBuf([]byte("abc"))); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	if string(sink.writes[0].Bytes()) != string([]byte{3}) || string(sink.writes[1].Bytes()) != "abc" {
		t.Fatalf("writes=%q,%q", sink.writes[0].Bytes(), sink.writes[1].Bytes())
	}
}

func TestProtobufVarint32Aliases(t *testing.T) {
	decoder, err := NewProtobufVarint32FrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	if decoder == nil {
		t.Fatal("decoder should not be nil")
	}
	if NewProtobufVarint32LengthFieldPrepender() == nil {
		t.Fatal("prepender should not be nil")
	}
}

func TestVarint32FrameDecoderRejectsMalformedHeader(t *testing.T) {
	decoder, err := NewVarint32FrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &errorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte{0x80, 0x80, 0x80, 0x80, 0x80}))
	if len(collector.errs) != 1 || !errors.Is(collector.errs[0], codec.ErrInvalidLengthField) {
		t.Fatalf("errs=%v, want invalid length field", collector.errs)
	}
}

func TestVarint32FrameDecoderReportsTooLongFrame(t *testing.T) {
	decoder, err := NewVarint32FrameDecoder(2)
	if err != nil {
		t.Fatal(err)
	}
	collector := &errorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte{3, 'a', 'b', 'c'}))
	if len(collector.errs) != 1 || !errors.Is(collector.errs[0], codec.ErrFrameTooLong) {
		t.Fatalf("errs=%v, want frame too long", collector.errs)
	}
}

type outboundSink struct {
	writes []buffer.ByteBuf
}

type errorCollector struct {
	errs []error
}

func (c *errorCollector) ExceptionCaught(_ *channel.HandlerContext, err error) {
	c.errs = append(c.errs, err)
}

func (s *outboundSink) Write(msg any) error {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		s.writes = append(s.writes, buf)
	}
	return nil
}

func (s *outboundSink) Flush() error { return nil }
func (s *outboundSink) Close() error { return nil }
func (s *outboundSink) release() {
	for _, buf := range s.writes {
		buf.Release()
	}
}

func testBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

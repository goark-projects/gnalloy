package codec

import (
	"testing"

	"github.com/goark-projects/gnalloy/buffer"
	"github.com/goark-projects/gnalloy/channel"
)

type frameCollector struct {
	frames []buffer.ByteBuf
	errs   []error
}

func (c *frameCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	if frame, ok := msg.(buffer.ByteBuf); ok {
		c.frames = append(c.frames, frame)
	}
}

func (c *frameCollector) ExceptionCaught(_ *channel.HandlerContext, err error) {
	c.errs = append(c.errs, err)
}

func TestLengthFieldDecoderSplitFrame(t *testing.T) {
	decoder, err := NewLengthFieldBasedFrameDecoder(1024, 0, 4, 0, 4, buffer.BigEndian)
	if err != nil {
		t.Fatal(err)
	}
	collector := &frameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("decoder", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(testBuf([]byte{0, 0, 0, 5, 'h'}))
	if len(collector.frames) != 0 {
		t.Fatal("split frame should not emit")
	}
	ch.Pipeline().FireChannelRead(testBuf([]byte{'e', 'l', 'l', 'o'}))
	if len(collector.frames) != 1 {
		t.Fatalf("frames=%d", len(collector.frames))
	}
	if string(collector.frames[0].Bytes()) != "hello" {
		t.Fatalf("frame=%q", collector.frames[0].Bytes())
	}
	collector.release()
}

func TestLengthFieldDecoderStickyFrames(t *testing.T) {
	decoder, err := NewLengthFieldBasedFrameDecoder(1024, 0, 4, 0, 4, buffer.BigEndian)
	if err != nil {
		t.Fatal(err)
	}
	collector := &frameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte{
		0, 0, 0, 4, 't', 'e', 's', 't',
		0, 0, 0, 3, 'a', 'b', 'c',
	}))
	if len(collector.frames) != 2 {
		t.Fatalf("frames=%d", len(collector.frames))
	}
	if string(collector.frames[0].Bytes()) != "test" || string(collector.frames[1].Bytes()) != "abc" {
		t.Fatalf("frames=%q,%q", collector.frames[0].Bytes(), collector.frames[1].Bytes())
	}
	collector.release()
}

func TestLengthFieldDecoderTooLong(t *testing.T) {
	decoder, err := NewLengthFieldBasedFrameDecoder(4, 0, 4, 0, 4, buffer.BigEndian)
	if err != nil {
		t.Fatal(err)
	}
	collector := &frameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte{0, 0, 0, 5, 'h', 'e', 'l', 'l', 'o'}))
	if len(collector.errs) != 1 || collector.errs[0] != ErrFrameTooLong {
		t.Fatalf("errs=%v", collector.errs)
	}
}

func (c *frameCollector) release() {
	for _, frame := range c.frames {
		frame.Release()
	}
	c.frames = nil
}

func testBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

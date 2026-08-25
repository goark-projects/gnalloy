package mqtt

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

type frameCollector struct {
	frames []buffer.ByteBuf
}

func (c *frameCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		c.frames = append(c.frames, buf)
	}
}

type mqttFrameCollector struct {
	frames []Frame
}

func (c *mqttFrameCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	if frame, ok := msg.(Frame); ok {
		c.frames = append(c.frames, frame)
	}
}

func TestFrameDecoder(t *testing.T) {
	decoder, err := NewFrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &frameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte{0x30, 0x03, 'a'}))
	ch.Pipeline().FireChannelRead(testBuf([]byte{'b', 'c'}))
	if len(collector.frames) != 1 || string(collector.frames[0].Bytes()) != string([]byte{0x30, 0x03, 'a', 'b', 'c'}) {
		t.Fatalf("frames=%v", collector.frames)
	}
	collector.frames[0].Release()
}

func TestFramePrepender(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("prepender", NewFramePrepender())
	defer sink.release()

	if err := ch.Write(Frame{TypeFlags: 0x30, Payload: testBuf([]byte("abc"))}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 || string(sink.writes[0].Bytes()) != string([]byte{0x30, 0x03}) || string(sink.writes[1].Bytes()) != "abc" {
		t.Fatalf("writes=%v", sink.writes)
	}
}

func TestTypedFrameDecoder(t *testing.T) {
	decoder, err := NewFrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &mqttFrameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("frame", decoder)
	_ = ch.Pipeline().AddLast("typed", NewTypedFrameDecoder())
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte{0x30, 0x03, 'a'}))
	ch.Pipeline().FireChannelRead(testBuf([]byte{'b', 'c'}))
	if len(collector.frames) != 1 {
		t.Fatalf("frames=%d, want 1", len(collector.frames))
	}
	frame := collector.frames[0]
	defer frame.Release()
	if frame.PacketType() != PacketPublish || frame.Flags() != 0 || string(frame.Payload.Bytes()) != "abc" {
		t.Fatalf("frame=%+v payload=%q", frame, frame.Payload.Bytes())
	}
}

func TestFramePrependerAllowsEmptyPayload(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("prepender", NewFramePrepender())
	defer sink.release()

	if err := ch.Write(PingResp()); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 || string(sink.writes[0].Bytes()) != string([]byte{0xd0, 0x00}) {
		t.Fatalf("writes=%v", sink.writes)
	}
}

type outboundSink struct {
	writes []buffer.ByteBuf
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

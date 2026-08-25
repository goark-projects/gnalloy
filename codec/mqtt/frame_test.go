package mqtt

import (
	"errors"
	"strings"
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

type mqttFrameCollector struct {
	frames []Frame
}

func (c *mqttFrameCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	if frame, ok := msg.(Frame); ok {
		c.frames = append(c.frames, frame)
	}
}

type packetCollector struct {
	packets []any
}

func (c *packetCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	c.packets = append(c.packets, msg)
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

func TestPacketDecoderDecodesConnect(t *testing.T) {
	collector := &packetCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("packet", NewPacketDecoder())
	_ = ch.Pipeline().AddLast("collector", collector)

	payload := testBuf([]byte{
		0, 4, 'M', 'Q', 'T', 'T',
		4, 2, 0, 30,
		0, 3, 'c', 'i', 'd',
	})
	ch.Pipeline().FireChannelRead(NewFrame(PacketConnect, 0, payload))
	if len(collector.packets) != 1 {
		t.Fatalf("packets=%d, want 1", len(collector.packets))
	}
	packet, ok := collector.packets[0].(ConnectPacket)
	if !ok {
		t.Fatalf("packet type=%T, want ConnectPacket", collector.packets[0])
	}
	if packet.ProtocolName != "MQTT" || packet.ProtocolLevel != 4 || packet.ClientID != "cid" || !packet.CleanSession || packet.KeepAliveSeconds != 30 {
		t.Fatalf("packet=%+v", packet)
	}
}

func TestPacketDecoderDecodesPublishAndRetainsPayload(t *testing.T) {
	collector := &packetCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("packet", NewPacketDecoder())
	_ = ch.Pipeline().AddLast("collector", collector)

	payload := testBuf([]byte{0, 1, 'a', 'b', 'c'})
	ch.Pipeline().FireChannelRead(NewFrame(PacketPublish, 0, payload))
	if len(collector.packets) != 1 {
		t.Fatalf("packets=%d, want 1", len(collector.packets))
	}
	packet, ok := collector.packets[0].(PublishPacket)
	if !ok {
		t.Fatalf("packet type=%T, want PublishPacket", collector.packets[0])
	}
	defer packet.Release()
	if packet.Topic != "a" || string(packet.Payload.Bytes()) != "bc" {
		t.Fatalf("packet=%+v payload=%q", packet, packet.Payload.Bytes())
	}
}

func TestPacketEncoderWritesPublishFrame(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("prepender", NewFramePrepender())
	_ = ch.Pipeline().AddLast("packet", NewPacketEncoder())
	defer sink.release()

	if err := ch.Write(PublishPacket{Topic: "a", Payload: testBuf([]byte("bc"))}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	if string(sink.writes[0].Bytes()) != string([]byte{0x30, 0x05}) {
		t.Fatalf("header=%v", sink.writes[0].Bytes())
	}
	if string(sink.writes[1].Bytes()) != string([]byte{0, 1, 'a', 'b', 'c'}) {
		t.Fatalf("payload=%v", sink.writes[1].Bytes())
	}
}

func TestPacketEncoderWritesSubscribeFrame(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("prepender", NewFramePrepender())
	_ = ch.Pipeline().AddLast("packet", NewPacketEncoder())
	defer sink.release()

	err := ch.Write(SubscribePacket{
		PacketID:      7,
		Subscriptions: []Subscription{{Topic: "sensor/1", QoS: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	if string(sink.writes[0].Bytes()) != string([]byte{0x82, 0x0d}) {
		t.Fatalf("header=%v", sink.writes[0].Bytes())
	}
	if string(sink.writes[1].Bytes()) != string([]byte{0, 7, 0, 8, 's', 'e', 'n', 's', 'o', 'r', '/', '1', 1}) {
		t.Fatalf("payload=%v", sink.writes[1].Bytes())
	}
}

func TestPacketEncoderRejectsOversizedPublishTopic(t *testing.T) {
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("packet", NewPacketEncoder())

	payload := testBuf([]byte("x"))
	err := ch.Write(PublishPacket{Topic: strings.Repeat("a", 65536), Payload: payload})
	if !errors.Is(err, codec.ErrInvalidFrameLength) {
		t.Fatalf("err=%v, want ErrInvalidFrameLength", err)
	}
	if payload.RefCnt() != 0 {
		t.Fatalf("payload ref=%d, want released", payload.RefCnt())
	}
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

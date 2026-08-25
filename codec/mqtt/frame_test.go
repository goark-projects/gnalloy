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

func TestPacketDecoderRejectsQoS1PublishWithZeroPacketID(t *testing.T) {
	payload := testBuf([]byte{0, 1, 'a', 0, 0, 'x'})
	defer payload.Release()

	_, err := DecodePacket(NewFrame(PacketPublish, 2, payload))
	if !errors.Is(err, codec.ErrInvalidFrameLength) {
		t.Fatalf("err=%v, want ErrInvalidFrameLength", err)
	}
}

func TestPacketDecoderDecodesUnsubscribe(t *testing.T) {
	collector := &packetCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("packet", NewPacketDecoder())
	_ = ch.Pipeline().AddLast("collector", collector)

	payload := testBuf([]byte{0, 7, 0, 8, 's', 'e', 'n', 's', 'o', 'r', '/', '1'})
	ch.Pipeline().FireChannelRead(NewFrame(PacketUnsubscribe, 2, payload))
	if len(collector.packets) != 1 {
		t.Fatalf("packets=%d, want 1", len(collector.packets))
	}
	packet, ok := collector.packets[0].(UnsubscribePacket)
	if !ok {
		t.Fatalf("packet type=%T, want UnsubscribePacket", collector.packets[0])
	}
	if packet.PacketID != 7 || len(packet.Topics) != 1 || packet.Topics[0] != "sensor/1" {
		t.Fatalf("packet=%+v", packet)
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

func TestPacketEncoderRejectsQoS1PublishWithoutPacketID(t *testing.T) {
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("packet", NewPacketEncoder())

	payload := testBuf([]byte("x"))
	err := ch.Write(PublishPacket{Topic: "a", QoS: QoSAtLeastOnce.Byte(), Payload: payload})
	if !errors.Is(err, codec.ErrInvalidFrameLength) {
		t.Fatalf("err=%v, want ErrInvalidFrameLength", err)
	}
	if payload.RefCnt() != 0 {
		t.Fatalf("payload ref=%d, want released", payload.RefCnt())
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

func TestPacketEncoderWritesUnsubscribeFrame(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("prepender", NewFramePrepender())
	_ = ch.Pipeline().AddLast("packet", NewPacketEncoder())
	defer sink.release()

	err := ch.Write(UnsubscribePacket{PacketID: 7, Topics: []string{"sensor/1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	if string(sink.writes[0].Bytes()) != string([]byte{0xa2, 0x0c}) {
		t.Fatalf("header=%v", sink.writes[0].Bytes())
	}
	if string(sink.writes[1].Bytes()) != string([]byte{0, 7, 0, 8, 's', 'e', 'n', 's', 'o', 'r', '/', '1'}) {
		t.Fatalf("payload=%v", sink.writes[1].Bytes())
	}
}

func TestPacketEncoderWritesUnsubAckFrame(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("prepender", NewFramePrepender())
	_ = ch.Pipeline().AddLast("packet", NewPacketEncoder())
	defer sink.release()

	if err := ch.Write(UnsubAckPacket{PacketID: 7}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	if string(sink.writes[0].Bytes()) != string([]byte{0xb0, 0x02}) {
		t.Fatalf("header=%v", sink.writes[0].Bytes())
	}
	if string(sink.writes[1].Bytes()) != string([]byte{0, 7}) {
		t.Fatalf("payload=%v", sink.writes[1].Bytes())
	}
}

func TestSubscribeDecoderRejectsReservedOptions(t *testing.T) {
	payload := testBuf([]byte{0, 7, 0, 1, 'a', 0x04})
	defer payload.Release()

	_, err := DecodePacket(NewFrame(PacketSubscribe, 2, payload))
	if !errors.Is(err, codec.ErrInvalidFrameLength) {
		t.Fatalf("err=%v, want ErrInvalidFrameLength", err)
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

func TestConnectFrameWritesMQTT5PropertyLength(t *testing.T) {
	var ctx *channel.HandlerContext
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("capture", handlerAddedFunc(func(c *channel.HandlerContext) error {
		ctx = c
		return nil
	}))

	frame, err := NewConnectFrame(ctx, ConnectPacket{
		ProtocolLevel:    ProtocolVersion5.Byte(),
		ClientID:         "c",
		CleanSession:     true,
		KeepAliveSeconds: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer frame.Release()
	want := []byte{0, 4, 'M', 'Q', 'T', 'T', 5, 2, 0, 30, 0, 0, 1, 'c'}
	if string(frame.Payload.Bytes()) != string(want) {
		t.Fatalf("payload=%v, want %v", frame.Payload.Bytes(), want)
	}
}

func TestMQTT5PropertiesEmpty(t *testing.T) {
	var empty MQTT5Properties
	if !empty.Empty() {
		t.Fatalf("empty properties should be empty")
	}
	props := MQTT5Properties{ReasonString: "bye"}
	if props.Empty() {
		t.Fatalf("properties with reason string should not be empty")
	}
}

func TestMQTT5ConnectPropertiesRoundTrip(t *testing.T) {
	ctx := mqttTestContext(t)
	frame, err := NewConnectFrame(ctx, ConnectPacket{
		ProtocolLevel:    ProtocolVersion5.Byte(),
		ClientID:         "cid",
		CleanSession:     true,
		KeepAliveSeconds: 30,
		Properties: MQTT5Properties{
			SessionExpiryInterval:    60,
			HasSessionExpiryInterval: true,
			UserProperties:           []UserProperty{{Key: "k", Value: "v"}},
		},
		WillFlag:    true,
		WillTopic:   "will/topic",
		WillPayload: []byte("payload"),
		WillProperties: MQTT5Properties{
			WillDelayInterval:    10,
			HasWillDelayInterval: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer frame.Release()

	packet, err := DecodePacket(frame)
	if err != nil {
		t.Fatal(err)
	}
	connect := packet.(ConnectPacket)
	if connect.ProtocolLevel != ProtocolVersion5.Byte() || connect.Properties.SessionExpiryInterval != 60 || len(connect.Properties.UserProperties) != 1 {
		t.Fatalf("connect=%+v", connect)
	}
	if !connect.WillFlag || connect.WillProperties.WillDelayInterval != 10 || connect.WillTopic != "will/topic" || string(connect.WillPayload) != "payload" {
		t.Fatalf("will=%+v", connect)
	}
}

func TestMQTT5PublishPropertiesRoundTrip(t *testing.T) {
	ctx := mqttTestContext(t)
	payload := testBuf([]byte("bc"))
	frame, err := NewPublishFrame(ctx, PublishPacket{
		ProtocolLevel: ProtocolVersion5.Byte(),
		Topic:         "a",
		Payload:       payload,
		Properties: MQTT5Properties{
			PayloadFormatIndicator:    1,
			HasPayloadFormatIndicator: true,
			ContentType:               "text/plain",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer frame.Release()

	packet, err := DecodePacketWithVersion(frame, ProtocolVersion5)
	if err != nil {
		t.Fatal(err)
	}
	publish := packet.(PublishPacket)
	defer publish.Release()
	if publish.ProtocolLevel != ProtocolVersion5.Byte() || publish.Topic != "a" || publish.Properties.PayloadFormatIndicator != 1 || publish.Properties.ContentType != "text/plain" {
		t.Fatalf("publish=%+v", publish)
	}
	if string(publish.Payload.Bytes()) != "bc" {
		t.Fatalf("payload=%q", publish.Payload.Bytes())
	}
}

func TestMQTT5AckReasonAndPropertiesRoundTrip(t *testing.T) {
	ctx := mqttTestContext(t)
	frame, err := newPacketIDFrame(ctx, PacketIDPacket{
		Type:          PacketPubAck,
		PacketID:      7,
		ReasonCode:    ReasonPacketIdentifierInUse.Byte(),
		ProtocolLevel: ProtocolVersion5.Byte(),
		Properties:    MQTT5Properties{ReasonString: "busy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer frame.Release()

	packet, err := DecodePacketWithVersion(frame, ProtocolVersion5)
	if err != nil {
		t.Fatal(err)
	}
	ack := packet.(PacketIDPacket)
	if ack.PacketID != 7 || ack.ReasonCode != ReasonPacketIdentifierInUse.Byte() || ack.Properties.ReasonString != "busy" {
		t.Fatalf("ack=%+v", ack)
	}
}

func TestMQTT5AuthRoundTrip(t *testing.T) {
	ctx := mqttTestContext(t)
	frame, err := NewAuthFrame(ctx, AuthPacket{
		ReasonCode: ReasonSuccess.Byte(),
		Properties: MQTT5Properties{
			AuthenticationMethod: "token",
			AuthenticationData:   []byte("abc"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer frame.Release()

	packet, err := DecodePacketWithVersion(frame, ProtocolVersion5)
	if err != nil {
		t.Fatal(err)
	}
	auth := packet.(AuthPacket)
	if auth.Properties.AuthenticationMethod != "token" || string(auth.Properties.AuthenticationData) != "abc" {
		t.Fatalf("auth=%+v", auth)
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

func mqttTestContext(t *testing.T) *channel.HandlerContext {
	t.Helper()
	var ctx *channel.HandlerContext
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("capture", handlerAddedFunc(func(c *channel.HandlerContext) error {
		ctx = c
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	if ctx == nil {
		t.Fatal("missing handler context")
	}
	return ctx
}

type handlerAddedFunc func(*channel.HandlerContext) error

func (f handlerAddedFunc) HandlerAdded(ctx *channel.HandlerContext) error {
	return f(ctx)
}

package mqtt

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

const (
	mqttProtocolName    = "MQTT"
	mqttProtocolLevel31 = 4
)

type ConnectPacket struct {
	ProtocolName     string
	ProtocolLevel    byte
	ClientID         string
	CleanSession     bool
	KeepAliveSeconds uint16
	WillFlag         bool
	WillQoS          byte
	WillRetain       bool
	WillTopic        string
	WillPayload      []byte
	Username         string
	Password         []byte
}

type ConnAckPacket struct {
	SessionPresent bool
	ReturnCode     byte
}

type PublishPacket struct {
	Dup      bool
	QoS      byte
	Retain   bool
	Topic    string
	PacketID uint16
	Payload  buffer.ByteBuf
}

func (p PublishPacket) Release() {
	if p.Payload != nil {
		p.Payload.Release()
	}
}

type PacketIDPacket struct {
	Type     byte
	PacketID uint16
}

type Subscription struct {
	Topic string
	QoS   byte
}

type SubscribePacket struct {
	PacketID      uint16
	Subscriptions []Subscription
}

type SubAckPacket struct {
	PacketID    uint16
	ReturnCodes []byte
}

type PingReqPacket struct{}
type PingRespPacket struct{}
type DisconnectPacket struct{}

type PacketDecoder struct {
	*codec.MessageToMessageDecoder
}

func NewPacketDecoder() *PacketDecoder {
	d := &PacketDecoder{}
	d.MessageToMessageDecoder = codec.NewMessageToMessageDecoder(d)
	return d
}

func (d *PacketDecoder) AcceptInboundMessage(msg any) bool {
	_, ok := msg.(Frame)
	return ok
}

func (d *PacketDecoder) Decode(_ *channel.HandlerContext, msg any, out *codec.MessageList) error {
	packet, err := DecodePacket(msg.(Frame))
	if err != nil {
		return err
	}
	out.Add(packet)
	return nil
}

type PacketEncoder struct{}

func NewPacketEncoder() *PacketEncoder {
	return &PacketEncoder{}
}

func (e *PacketEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	frame, ok, err := EncodePacket(ctx, msg)
	if err != nil {
		return err
	}
	if !ok {
		return ctx.Write(msg)
	}
	if err := ctx.Write(frame); err != nil {
		frame.Release()
		return err
	}
	return nil
}

func DecodePacket(frame Frame) (any, error) {
	switch frame.PacketType() {
	case PacketConnect:
		return decodeConnect(frame)
	case PacketConnAck:
		return decodeConnAck(frame)
	case PacketPublish:
		return decodePublish(frame)
	case PacketPubAck, PacketPubRec, PacketPubRel, PacketPubComp, PacketUnsubAck:
		return decodePacketID(frame)
	case PacketSubscribe:
		return decodeSubscribe(frame)
	case PacketSubAck:
		return decodeSubAck(frame)
	case PacketPingReq:
		if frame.Flags() != 0 || frame.RemainingLength() != 0 {
			return nil, codec.ErrInvalidFrameLength
		}
		return PingReqPacket{}, nil
	case PacketPingResp:
		if frame.Flags() != 0 || frame.RemainingLength() != 0 {
			return nil, codec.ErrInvalidFrameLength
		}
		return PingRespPacket{}, nil
	case PacketDisconnect:
		if frame.Flags() != 0 || frame.RemainingLength() != 0 {
			return nil, codec.ErrInvalidFrameLength
		}
		return DisconnectPacket{}, nil
	default:
		return nil, codec.ErrInvalidFrameLength
	}
}

func EncodePacket(ctx *channel.HandlerContext, msg any) (Frame, bool, error) {
	switch packet := msg.(type) {
	case ConnectPacket:
		frame, err := NewConnectFrame(ctx, packet)
		return frame, true, err
	case ConnAckPacket:
		frame, err := NewConnAckFrame(ctx, packet)
		return frame, true, err
	case PublishPacket:
		frame, err := NewPublishFrame(ctx, packet)
		return frame, true, err
	case PacketIDPacket:
		frame, err := NewPacketIDFrame(ctx, packet.Type, packet.PacketID)
		return frame, true, err
	case SubscribePacket:
		frame, err := NewSubscribeFrame(ctx, packet)
		return frame, true, err
	case SubAckPacket:
		frame, err := NewSubAckFrame(ctx, packet)
		return frame, true, err
	case PingReqPacket:
		return NewFrame(PacketPingReq, 0, nil), true, nil
	case PingRespPacket:
		return NewFrame(PacketPingResp, 0, nil), true, nil
	case DisconnectPacket:
		return NewFrame(PacketDisconnect, 0, nil), true, nil
	default:
		return Frame{}, false, nil
	}
}

func NewConnectFrame(ctx *channel.HandlerContext, packet ConnectPacket) (Frame, error) {
	protocolName := packet.ProtocolName
	if protocolName == "" {
		protocolName = mqttProtocolName
	}
	protocolLevel := packet.ProtocolLevel
	if protocolLevel == 0 {
		protocolLevel = mqttProtocolLevel31
	}
	if packet.WillQoS > 2 || (!packet.WillFlag && (packet.WillQoS != 0 || packet.WillRetain)) {
		return Frame{}, codec.ErrInvalidFrameLength
	}
	if !validMQTTString(protocolName) || !validMQTTString(packet.ClientID) || !validMQTTString(packet.Username) || !validMQTTBinary(packet.Password) {
		return Frame{}, codec.ErrInvalidFrameLength
	}
	if packet.WillFlag && (!validMQTTString(packet.WillTopic) || !validMQTTBinary(packet.WillPayload)) {
		return Frame{}, codec.ErrInvalidFrameLength
	}
	size := mqttStringSize(protocolName) + 4 + mqttStringSize(packet.ClientID)
	if packet.WillFlag {
		size += mqttStringSize(packet.WillTopic) + mqttBinarySize(packet.WillPayload)
	}
	if packet.Username != "" {
		size += mqttStringSize(packet.Username)
	}
	if packet.Password != nil {
		size += mqttBinarySize(packet.Password)
	}
	payload, err := ctx.Channel().Allocator().Acquire(size)
	if err != nil {
		return Frame{}, err
	}
	if err := writeMQTTString(payload, protocolName); err != nil {
		payload.Release()
		return Frame{}, err
	}
	flags := byte(0)
	if packet.Username != "" {
		flags |= 0x80
	}
	if packet.Password != nil {
		flags |= 0x40
	}
	if packet.WillRetain {
		flags |= 0x20
	}
	flags |= (packet.WillQoS & 0x03) << 3
	if packet.WillFlag {
		flags |= 0x04
	}
	if packet.CleanSession {
		flags |= 0x02
	}
	if _, err := payload.WriteBytes([]byte{protocolLevel, flags, byte(packet.KeepAliveSeconds >> 8), byte(packet.KeepAliveSeconds)}); err != nil {
		payload.Release()
		return Frame{}, err
	}
	if err := writeMQTTString(payload, packet.ClientID); err != nil {
		payload.Release()
		return Frame{}, err
	}
	if packet.WillFlag {
		if err := writeMQTTString(payload, packet.WillTopic); err != nil {
			payload.Release()
			return Frame{}, err
		}
		if err := writeMQTTBinary(payload, packet.WillPayload); err != nil {
			payload.Release()
			return Frame{}, err
		}
	}
	if packet.Username != "" {
		if err := writeMQTTString(payload, packet.Username); err != nil {
			payload.Release()
			return Frame{}, err
		}
	}
	if packet.Password != nil {
		if err := writeMQTTBinary(payload, packet.Password); err != nil {
			payload.Release()
			return Frame{}, err
		}
	}
	return NewFrame(PacketConnect, 0, payload), nil
}

func NewConnAckFrame(ctx *channel.HandlerContext, packet ConnAckPacket) (Frame, error) {
	payload, err := ctx.Channel().Allocator().Acquire(2)
	if err != nil {
		return Frame{}, err
	}
	ackFlags := byte(0)
	if packet.SessionPresent {
		ackFlags = 1
	}
	if _, err := payload.WriteBytes([]byte{ackFlags, packet.ReturnCode}); err != nil {
		payload.Release()
		return Frame{}, err
	}
	return NewFrame(PacketConnAck, 0, payload), nil
}

func NewPublishFrame(ctx *channel.HandlerContext, packet PublishPacket) (Frame, error) {
	if packet.Topic == "" || !validMQTTString(packet.Topic) || packet.QoS > 2 || (packet.QoS == 0 && packet.PacketID != 0) {
		if packet.Payload != nil {
			packet.Payload.Release()
		}
		return Frame{}, codec.ErrInvalidFrameLength
	}
	headSize := mqttStringSize(packet.Topic)
	if packet.QoS > 0 {
		headSize += 2
	}
	head, err := ctx.Channel().Allocator().Acquire(headSize)
	if err != nil {
		if packet.Payload != nil {
			packet.Payload.Release()
		}
		return Frame{}, err
	}
	if err := writeMQTTString(head, packet.Topic); err != nil {
		head.Release()
		if packet.Payload != nil {
			packet.Payload.Release()
		}
		return Frame{}, err
	}
	if packet.QoS > 0 {
		if packet.PacketID == 0 {
			head.Release()
			if packet.Payload != nil {
				packet.Payload.Release()
			}
			return Frame{}, codec.ErrInvalidFrameLength
		}
		if err := writeUint16(head, packet.PacketID); err != nil {
			head.Release()
			if packet.Payload != nil {
				packet.Payload.Release()
			}
			return Frame{}, err
		}
	}
	payload := buffer.NewCompositeByteBuf()
	payload.Append(head)
	if packet.Payload != nil {
		payload.Append(packet.Payload)
	}
	flags := byte(0)
	if packet.Dup {
		flags |= 0x08
	}
	flags |= (packet.QoS & 0x03) << 1
	if packet.Retain {
		flags |= 0x01
	}
	return NewFrame(PacketPublish, flags, payload), nil
}

func NewPacketIDFrame(ctx *channel.HandlerContext, packetType byte, packetID uint16) (Frame, error) {
	if packetID == 0 {
		return Frame{}, codec.ErrInvalidFrameLength
	}
	flags := byte(0)
	if packetType == PacketPubRel || packetType == PacketSubscribe || packetType == PacketUnsubscribe {
		flags = 2
	}
	payload, err := ctx.Channel().Allocator().Acquire(2)
	if err != nil {
		return Frame{}, err
	}
	if err := writeUint16(payload, packetID); err != nil {
		payload.Release()
		return Frame{}, err
	}
	return NewFrame(packetType, flags, payload), nil
}

func NewSubscribeFrame(ctx *channel.HandlerContext, packet SubscribePacket) (Frame, error) {
	if packet.PacketID == 0 || len(packet.Subscriptions) == 0 {
		return Frame{}, codec.ErrInvalidFrameLength
	}
	size := 2
	for _, sub := range packet.Subscriptions {
		if sub.Topic == "" || !validMQTTString(sub.Topic) || sub.QoS > 2 {
			return Frame{}, codec.ErrInvalidFrameLength
		}
		size += mqttStringSize(sub.Topic) + 1
	}
	payload, err := ctx.Channel().Allocator().Acquire(size)
	if err != nil {
		return Frame{}, err
	}
	if err := writeUint16(payload, packet.PacketID); err != nil {
		payload.Release()
		return Frame{}, err
	}
	for _, sub := range packet.Subscriptions {
		if err := writeMQTTString(payload, sub.Topic); err != nil {
			payload.Release()
			return Frame{}, err
		}
		if _, err := payload.WriteBytes([]byte{sub.QoS}); err != nil {
			payload.Release()
			return Frame{}, err
		}
	}
	return NewFrame(PacketSubscribe, 2, payload), nil
}

func NewSubAckFrame(ctx *channel.HandlerContext, packet SubAckPacket) (Frame, error) {
	if packet.PacketID == 0 {
		return Frame{}, codec.ErrInvalidFrameLength
	}
	payload, err := ctx.Channel().Allocator().Acquire(2 + len(packet.ReturnCodes))
	if err != nil {
		return Frame{}, err
	}
	if err := writeUint16(payload, packet.PacketID); err != nil {
		payload.Release()
		return Frame{}, err
	}
	if _, err := payload.WriteBytes(packet.ReturnCodes); err != nil {
		payload.Release()
		return Frame{}, err
	}
	return NewFrame(PacketSubAck, 0, payload), nil
}

func PingReq() Frame {
	return NewFrame(PacketPingReq, 0, nil)
}

func Disconnect() Frame {
	return NewFrame(PacketDisconnect, 0, nil)
}

func decodeConnect(frame Frame) (ConnectPacket, error) {
	if frame.Flags() != 0 || frame.Payload == nil {
		return ConnectPacket{}, codec.ErrInvalidFrameLength
	}
	idx := frame.Payload.ReaderIndex()
	protocolName, next, err := readMQTTString(frame.Payload, idx)
	if err != nil {
		return ConnectPacket{}, err
	}
	if frame.Payload.WriterIndex()-next < 4 {
		return ConnectPacket{}, codec.ErrInvalidFrameLength
	}
	level, _ := frame.Payload.GetByte(next)
	flags, _ := frame.Payload.GetByte(next + 1)
	keepAlive, err := readUint16(frame.Payload, next+2)
	if err != nil {
		return ConnectPacket{}, err
	}
	if flags&0x01 != 0 {
		return ConnectPacket{}, codec.ErrInvalidFrameLength
	}
	willFlag := flags&0x04 != 0
	willQoS := (flags >> 3) & 0x03
	willRetain := flags&0x20 != 0
	if willQoS > 2 || (!willFlag && (willQoS != 0 || willRetain)) {
		return ConnectPacket{}, codec.ErrInvalidFrameLength
	}
	idx = next + 4
	clientID, idx, err := readMQTTString(frame.Payload, idx)
	if err != nil {
		return ConnectPacket{}, err
	}
	packet := ConnectPacket{
		ProtocolName:     protocolName,
		ProtocolLevel:    level,
		ClientID:         clientID,
		CleanSession:     flags&0x02 != 0,
		KeepAliveSeconds: keepAlive,
		WillFlag:         willFlag,
		WillQoS:          willQoS,
		WillRetain:       willRetain,
	}
	if willFlag {
		packet.WillTopic, idx, err = readMQTTString(frame.Payload, idx)
		if err != nil {
			return ConnectPacket{}, err
		}
		packet.WillPayload, idx, err = readMQTTBinary(frame.Payload, idx)
		if err != nil {
			return ConnectPacket{}, err
		}
	}
	if flags&0x80 != 0 {
		packet.Username, idx, err = readMQTTString(frame.Payload, idx)
		if err != nil {
			return ConnectPacket{}, err
		}
	}
	if flags&0x40 != 0 {
		packet.Password, idx, err = readMQTTBinary(frame.Payload, idx)
		if err != nil {
			return ConnectPacket{}, err
		}
	}
	if idx != frame.Payload.WriterIndex() {
		return ConnectPacket{}, codec.ErrInvalidFrameLength
	}
	return packet, nil
}

func decodeConnAck(frame Frame) (ConnAckPacket, error) {
	if frame.Flags() != 0 || frame.Payload == nil || frame.Payload.ReadableBytes() != 2 {
		return ConnAckPacket{}, codec.ErrInvalidFrameLength
	}
	idx := frame.Payload.ReaderIndex()
	flags, _ := frame.Payload.GetByte(idx)
	code, _ := frame.Payload.GetByte(idx + 1)
	if flags&0xfe != 0 {
		return ConnAckPacket{}, codec.ErrInvalidFrameLength
	}
	return ConnAckPacket{SessionPresent: flags&1 != 0, ReturnCode: code}, nil
}

func decodePublish(frame Frame) (PublishPacket, error) {
	flags := frame.Flags()
	qos := (flags >> 1) & 0x03
	if qos == 3 || frame.Payload == nil {
		return PublishPacket{}, codec.ErrInvalidFrameLength
	}
	topic, idx, err := readMQTTString(frame.Payload, frame.Payload.ReaderIndex())
	if err != nil {
		return PublishPacket{}, err
	}
	packet := PublishPacket{Dup: flags&0x08 != 0, QoS: qos, Retain: flags&0x01 != 0, Topic: topic}
	if qos > 0 {
		packet.PacketID, err = readUint16(frame.Payload, idx)
		if err != nil {
			return PublishPacket{}, err
		}
		idx += 2
	}
	if frame.Payload.WriterIndex() > idx {
		packet.Payload, err = frame.Payload.Slice(idx, frame.Payload.WriterIndex()-idx)
		if err != nil {
			return PublishPacket{}, err
		}
	}
	return packet, nil
}

func decodePacketID(frame Frame) (PacketIDPacket, error) {
	expectedFlags := byte(0)
	if frame.PacketType() == PacketPubRel {
		expectedFlags = 2
	}
	if frame.Flags() != expectedFlags || frame.Payload == nil || frame.Payload.ReadableBytes() != 2 {
		return PacketIDPacket{}, codec.ErrInvalidFrameLength
	}
	id, err := readUint16(frame.Payload, frame.Payload.ReaderIndex())
	if err != nil {
		return PacketIDPacket{}, err
	}
	if id == 0 {
		return PacketIDPacket{}, codec.ErrInvalidFrameLength
	}
	return PacketIDPacket{Type: frame.PacketType(), PacketID: id}, nil
}

func decodeSubscribe(frame Frame) (SubscribePacket, error) {
	if frame.Flags() != 2 || frame.Payload == nil || frame.Payload.ReadableBytes() < 5 {
		return SubscribePacket{}, codec.ErrInvalidFrameLength
	}
	idx := frame.Payload.ReaderIndex()
	id, err := readUint16(frame.Payload, idx)
	if err != nil || id == 0 {
		return SubscribePacket{}, codec.ErrInvalidFrameLength
	}
	idx += 2
	packet := SubscribePacket{PacketID: id}
	for idx < frame.Payload.WriterIndex() {
		topic, next, err := readMQTTString(frame.Payload, idx)
		if err != nil {
			return SubscribePacket{}, err
		}
		if frame.Payload.WriterIndex() <= next {
			return SubscribePacket{}, codec.ErrInvalidFrameLength
		}
		qos, _ := frame.Payload.GetByte(next)
		if topic == "" || qos > 2 {
			return SubscribePacket{}, codec.ErrInvalidFrameLength
		}
		packet.Subscriptions = append(packet.Subscriptions, Subscription{Topic: topic, QoS: qos})
		idx = next + 1
	}
	if len(packet.Subscriptions) == 0 {
		return SubscribePacket{}, codec.ErrInvalidFrameLength
	}
	return packet, nil
}

func decodeSubAck(frame Frame) (SubAckPacket, error) {
	if frame.Flags() != 0 || frame.Payload == nil || frame.Payload.ReadableBytes() < 2 {
		return SubAckPacket{}, codec.ErrInvalidFrameLength
	}
	id, err := readUint16(frame.Payload, frame.Payload.ReaderIndex())
	if err != nil || id == 0 {
		return SubAckPacket{}, codec.ErrInvalidFrameLength
	}
	start := frame.Payload.ReaderIndex() + 2
	codesLen := frame.Payload.WriterIndex() - start
	codes := make([]byte, codesLen)
	for i := 0; i < codesLen; i++ {
		codes[i], _ = frame.Payload.GetByte(start + i)
	}
	return SubAckPacket{PacketID: id, ReturnCodes: codes}, nil
}

func mqttStringSize(s string) int {
	return 2 + len(s)
}

func mqttBinarySize(data []byte) int {
	return 2 + len(data)
}

func validMQTTString(s string) bool {
	return len(s) <= 0xffff
}

func validMQTTBinary(data []byte) bool {
	return len(data) <= 0xffff
}

func writeMQTTString(buf buffer.ByteBuf, s string) error {
	if len(s) > 0xffff {
		return codec.ErrInvalidFrameLength
	}
	if err := writeUint16(buf, uint16(len(s))); err != nil {
		return err
	}
	_, err := buf.WriteBytes([]byte(s))
	return err
}

func writeMQTTBinary(buf buffer.ByteBuf, data []byte) error {
	if len(data) > 0xffff {
		return codec.ErrInvalidFrameLength
	}
	if err := writeUint16(buf, uint16(len(data))); err != nil {
		return err
	}
	_, err := buf.WriteBytes(data)
	return err
}

func readMQTTString(buf buffer.ByteBuf, index int) (string, int, error) {
	data, next, err := readMQTTBinary(buf, index)
	if err != nil {
		return "", 0, err
	}
	return string(data), next, nil
}

func readMQTTBinary(buf buffer.ByteBuf, index int) ([]byte, int, error) {
	length, err := readUint16(buf, index)
	if err != nil {
		return nil, 0, err
	}
	start := index + 2
	end := start + int(length)
	if end > buf.WriterIndex() {
		return nil, 0, codec.ErrInvalidFrameLength
	}
	out := make([]byte, int(length))
	for i := range out {
		out[i], _ = buf.GetByte(start + i)
	}
	return out, end, nil
}

func writeUint16(buf buffer.ByteBuf, value uint16) error {
	_, err := buf.WriteBytes([]byte{byte(value >> 8), byte(value)})
	return err
}

func readUint16(buf buffer.ByteBuf, index int) (uint16, error) {
	if index+2 > buf.WriterIndex() {
		return 0, codec.ErrInvalidFrameLength
	}
	hi, ok := buf.GetByte(index)
	if !ok {
		return 0, codec.ErrInvalidFrameLength
	}
	lo, ok := buf.GetByte(index + 1)
	if !ok {
		return 0, codec.ErrInvalidFrameLength
	}
	return uint16(hi)<<8 | uint16(lo), nil
}

package protocol

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/l2"
	"goark.dev/gnalloy/transport/raw"
	"goark.dev/gnalloy/transport/udp"
)

// Adapter 定义应用 payload 和具体传输消息之间的映射。
type Adapter interface {
	// BuildRequest 把应用 payload 转成可写入 Channel 的出站消息。
	BuildRequest(ch channel.Channel, payload []byte) (any, error)
	// MatchResponse 从入站消息中提取响应 payload；matched=false 表示继续等待。
	MatchResponse(request []byte, msg any) (payload []byte, matched bool, err error)
}

// StreamAdapter 适配 TCP、Unix、QUIC stream 等 ByteBuf 流式传输。
type StreamAdapter struct{}

// BuildRequest 把 payload 写入新的 ByteBuf。
func (StreamAdapter) BuildRequest(ch channel.Channel, payload []byte) (any, error) {
	return newPayloadBuffer(ch, payload)
}

// MatchResponse 提取 ByteBuf 响应。
func (StreamAdapter) MatchResponse(_ []byte, msg any) ([]byte, bool, error) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return nil, false, nil
	}
	return copyBufferBytes(buf), true, nil
}

// DatagramAdapter 适配 UDP connected endpoint 的 request-response。
type DatagramAdapter struct{}

// BuildRequest 使用 ByteBuf 让 connected UDP endpoint 绑定默认远端地址。
func (DatagramAdapter) BuildRequest(ch channel.Channel, payload []byte) (any, error) {
	return newPayloadBuffer(ch, payload)
}

// MatchResponse 提取 UDP Datagram 响应。
func (DatagramAdapter) MatchResponse(_ []byte, msg any) ([]byte, bool, error) {
	datagram, ok := msg.(udp.Datagram)
	if !ok || datagram.Payload == nil {
		return nil, false, nil
	}
	return copyBufferBytes(datagram.Payload), true, nil
}

// PacketAdapter 适配 raw connected endpoint 的自定义 IP 协议消息。
type PacketAdapter struct{}

// BuildRequest 使用 ByteBuf 让 connected raw endpoint 填充默认远端和协议。
func (PacketAdapter) BuildRequest(ch channel.Channel, payload []byte) (any, error) {
	return newPayloadBuffer(ch, payload)
}

// MatchResponse 提取 raw Packet 响应。
func (PacketAdapter) MatchResponse(_ []byte, msg any) ([]byte, bool, error) {
	packet, ok := msg.(raw.Packet)
	if !ok || packet.Payload == nil {
		return nil, false, nil
	}
	return copyBufferBytes(packet.Payload), true, nil
}

// FrameAdapter 适配 L2 frame endpoint 的二层帧消息。
type FrameAdapter struct{}

// BuildRequest 使用 ByteBuf 构造 L2 出站帧负载。
func (FrameAdapter) BuildRequest(ch channel.Channel, payload []byte) (any, error) {
	return newPayloadBuffer(ch, payload)
}

// MatchResponse 提取 L2 Frame 响应。
func (FrameAdapter) MatchResponse(_ []byte, msg any) ([]byte, bool, error) {
	frame, ok := msg.(l2.Frame)
	if !ok || frame.Payload == nil {
		return nil, false, nil
	}
	return copyBufferBytes(frame.Payload), true, nil
}

func newPayloadBuffer(ch channel.Channel, payload []byte) (buffer.ByteBuf, error) {
	if ch == nil || ch.Allocator() == nil {
		return nil, ErrInvalidConfig
	}
	buf, err := ch.Allocator().Acquire(len(payload))
	if err != nil {
		return nil, err
	}
	if _, err := buf.WriteBytes(payload); err != nil {
		buf.Release()
		return nil, err
	}
	return buf, nil
}

func copyBufferBytes(buf buffer.ByteBuf) []byte {
	if buf == nil || buf.ReadableBytes() == 0 {
		return nil
	}
	out := make([]byte, 0, buf.ReadableBytes())
	for _, part := range buf.ReadableSlices(nil) {
		out = append(out, part...)
	}
	return out
}

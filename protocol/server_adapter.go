package protocol

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/l2"
	"goark.dev/gnalloy/transport/raw"
	"goark.dev/gnalloy/transport/udp"
)

// ServerAdapter 定义入站传输消息和统一 Request/Response 之间的映射。
type ServerAdapter interface {
	// ExtractRequest 从入站消息中提取应用请求；matched=false 表示交给后续 handler。
	ExtractRequest(ch channel.Channel, msg any) (Request, bool, error)
	// BuildResponse 把应用响应负载转换为可写回 Channel 的出站消息。
	BuildResponse(ch channel.Channel, req Request, payload []byte) (any, error)
}

// ExtractRequest 提取 ByteBuf 请求。
func (StreamAdapter) ExtractRequest(ch channel.Channel, msg any) (Request, bool, error) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return Request{}, false, nil
	}
	return Request{Channel: ch, Payload: copyBufferBytes(buf), Message: msg}, true, nil
}

// BuildResponse 构造 ByteBuf 响应。
func (StreamAdapter) BuildResponse(ch channel.Channel, _ Request, payload []byte) (any, error) {
	return newPayloadBuffer(ch, payload)
}

// ExtractRequest 提取 UDP datagram 请求，并保留源地址用于响应。
func (DatagramAdapter) ExtractRequest(ch channel.Channel, msg any) (Request, bool, error) {
	datagram, ok := msg.(udp.Datagram)
	if !ok || datagram.Payload == nil {
		return Request{}, false, nil
	}
	return Request{
		Channel: ch,
		Payload: copyBufferBytes(datagram.Payload),
		Message: msg,
		Meta:    RequestMeta{DatagramAddr: datagram.Addr},
	}, true, nil
}

// BuildResponse 构造 UDP datagram 响应，目标地址复用请求源地址。
func (DatagramAdapter) BuildResponse(ch channel.Channel, req Request, payload []byte) (any, error) {
	buf, err := newPayloadBuffer(ch, payload)
	if err != nil {
		return nil, err
	}
	return udp.Datagram{Payload: buf, Addr: req.Meta.DatagramAddr}, nil
}

// ExtractRequest 提取 raw packet 请求，并保留源地址和协议号用于响应。
func (PacketAdapter) ExtractRequest(ch channel.Channel, msg any) (Request, bool, error) {
	packet, ok := msg.(raw.Packet)
	if !ok || packet.Payload == nil {
		return Request{}, false, nil
	}
	return Request{
		Channel: ch,
		Payload: copyBufferBytes(packet.Payload),
		Message: msg,
		Meta: RequestMeta{
			PacketAddr:     packet.Addr,
			PacketProtocol: packet.Protocol,
		},
	}, true, nil
}

// BuildResponse 构造 raw packet 响应，目标地址和协议号复用请求元数据。
func (PacketAdapter) BuildResponse(ch channel.Channel, req Request, payload []byte) (any, error) {
	buf, err := newPayloadBuffer(ch, payload)
	if err != nil {
		return nil, err
	}
	return raw.Packet{
		Payload:  buf,
		Addr:     req.Meta.PacketAddr,
		Protocol: req.Meta.PacketProtocol,
	}, nil
}

// ExtractRequest 提取 L2 frame 请求，并保留二层元数据用于响应。
func (FrameAdapter) ExtractRequest(ch channel.Channel, msg any) (Request, bool, error) {
	frame, ok := msg.(l2.Frame)
	if !ok || frame.Payload == nil {
		return Request{}, false, nil
	}
	return Request{
		Channel: ch,
		Payload: copyBufferBytes(frame.Payload),
		Message: msg,
		Meta:    RequestMeta{FrameMeta: frame.Meta},
	}, true, nil
}

// BuildResponse 构造 L2 frame 响应；payload 仍表示完整二层帧字节。
func (FrameAdapter) BuildResponse(ch channel.Channel, req Request, payload []byte) (any, error) {
	buf, err := newPayloadBuffer(ch, payload)
	if err != nil {
		return nil, err
	}
	return l2.Frame{Meta: req.Meta.FrameMeta, Payload: buf}, nil
}

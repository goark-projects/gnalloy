package protocol

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/l2"
	"goark.dev/gnalloy/transport/raw"
	"goark.dev/gnalloy/transport/udp"
)

// Request 是应用协议服务端看到的统一请求。
//
// Payload 是从入站消息复制出的应用字节，调用方可在 handler 返回后继续持有；
// Message 保留原始传输消息，仅用于当前 handler 调用内的高级场景。
type Request struct {
	// Channel 是承载本次请求的 gnalloy Channel。
	Channel channel.Channel
	// Payload 是上层协议负载的独立副本。
	Payload []byte
	// Message 是原始入站消息，handler 返回后不得继续引用其内部缓冲区。
	Message any
	// Meta 保存具体传输携带的旁路元数据。
	Meta RequestMeta
}

// RequestMeta 保存 stream、datagram、raw packet、L2 frame 的统一元数据。
type RequestMeta struct {
	// DatagramAddr 是 UDP datagram 的源地址。
	DatagramAddr udp.Address
	// PacketAddr 是 raw packet 的源地址。
	PacketAddr raw.Address
	// PacketProtocol 是 raw packet 使用的 IP 协议号。
	PacketProtocol int
	// FrameMeta 是二层帧驱动解析出的元数据。
	FrameMeta l2.FrameMeta
}

package l2

import (
	"context"
	"net"

	"goark.dev/gnalloy/buffer"
)

// DriverKind 描述 native L2 driver 类型。
type DriverKind string

const (
	DriverKindUnknown  DriverKind = "unknown"
	DriverKindAFPacket DriverKind = "af_packet"
	DriverKindBPF      DriverKind = "bpf"
	DriverKindNpcap    DriverKind = "npcap"
)

// FrameMeta 描述二层帧的旁路元数据。
type FrameMeta struct {
	// InterfaceName 是收发帧使用的网卡名称。
	InterfaceName string
	// InterfaceIndex 是操作系统分配的网卡索引。
	InterfaceIndex int
	// Source 是以太网源 MAC 地址。
	Source net.HardwareAddr
	// Destination 是以太网目的 MAC 地址。
	Destination net.HardwareAddr
	// EtherType 是以太网帧的协议类型。
	EtherType uint16
}

// Frame 是 L2 Pipeline 的入站和出站消息，Payload 保存完整二层帧字节。
type Frame struct {
	// Meta 保存平台驱动解析出的旁路元数据。
	Meta FrameMeta
	// Payload 保存完整二层帧字节，生命周期由 Pipeline 消费方显式释放。
	Payload buffer.ByteBuf
}

// Valid 判断帧是否包含可读的二层负载。
func (f Frame) Valid() bool {
	return f.Payload != nil && f.Payload.ReadableBytes() > 0
}

// Release 释放帧持有的 ByteBuf。
func (f Frame) Release() {
	if f.Payload != nil {
		f.Payload.Release()
	}
}

// Driver 打开平台二层收发 Endpoint。
type Driver interface {
	// Open 根据配置打开一个平台二层收发端。
	Open(ctx context.Context, cfg Config) (Endpoint, error)
}

// Endpoint 是 Driver 返回的二层帧读写端。
type Endpoint interface {
	// Addr 返回 endpoint 绑定的接口地址描述。
	Addr() string
	// ReadFrame 读取一帧数据；返回的 Frame 由上层 Pipeline 消费方负责释放。
	ReadFrame(ctx context.Context, alloc buffer.Allocator, readBufferSize int) (Frame, error)
	// WriteFrame 写出一帧数据；实现不得接管 Frame 生命周期，若需异步持有必须自行复制。
	WriteFrame(ctx context.Context, frame Frame) error
	// Close 关闭底层平台资源。
	Close() error
}

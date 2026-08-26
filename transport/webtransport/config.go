package webtransport

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

const (
	defaultReadBufferSize = 4096
	defaultChannelIDBase  = 1
)

// Config 描述 WebTransport stream channel 与 datagram 的本地绑定参数。
type Config struct {
	// Allocator 为每个 WebTransport stream channel 提供 ByteBuf。
	Allocator buffer.Allocator
	// ReadBufferSize 控制单次从 QUIC stream 读取的缓冲区大小。
	ReadBufferSize int
	// ChannelIDBase 控制自动生成的 ChannelID 起点，0 表示从 1 开始。
	ChannelIDBase transport.ChannelID
	// MaxDatagramPayload 限制 WebTransport datagram payload 字节数，0 表示不额外限制。
	MaxDatagramPayload int
	// DisableCapabilityValidation 仅用于测试或外层已完成协商校验的场景。
	DisableCapabilityValidation bool
}

func normalizeConfig(cfg Config) Config {
	if cfg.Allocator == nil {
		cfg.Allocator = buffer.NewHeapAllocator()
	}
	if cfg.ReadBufferSize <= 0 {
		cfg.ReadBufferSize = defaultReadBufferSize
	}
	if cfg.ChannelIDBase == 0 {
		cfg.ChannelIDBase = defaultChannelIDBase
	}
	return cfg
}

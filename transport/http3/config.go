package http3

import (
	"goark.dev/gnalloy/buffer"
	codechttp3 "goark.dev/gnalloy/codec/http3"
	"goark.dev/gnalloy/transport"
)

const (
	defaultReadBufferSize = 4096
	defaultChannelIDBase  = 1
)

// Config 描述 HTTP/3 QUIC stream 到 gnalloy Channel 的绑定参数。
type Config struct {
	// Pipeline 是 HTTP/3 request/control/QPACK pipeline 的编解码配置。
	Pipeline codechttp3.PipelineConfig
	// Allocator 为每个 stream channel 提供 ByteBuf；为空时使用 HeapAllocator。
	Allocator buffer.Allocator
	// ReadBufferSize 控制单次从 QUIC stream 读取的缓冲区大小。
	ReadBufferSize int
	// ChannelIDBase 控制自动生成的 ChannelID 起点，0 表示从 1 开始。
	ChannelIDBase transport.ChannelID
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

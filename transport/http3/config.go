package http3

import (
	"strings"

	"goark.dev/gnalloy/buffer"
	codechttp3 "goark.dev/gnalloy/codec/http3"
	"goark.dev/gnalloy/transport"
)

const (
	defaultReadBufferSize = 4096
	defaultChannelIDBase  = 1
	defaultALPN           = "h3"
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
	// AllowedALPN 是允许绑定为 HTTP/3 的 QUIC TLS ALPN；为空时只接受 h3。
	AllowedALPN []string
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
	cfg.AllowedALPN = normalizeAllowedALPN(cfg.AllowedALPN)
	return cfg
}

func normalizeAllowedALPN(values []string) []string {
	if len(values) == 0 {
		return []string{defaultALPN}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		protocol := strings.TrimSpace(value)
		if protocol != "" {
			out = append(out, protocol)
		}
	}
	if len(out) == 0 {
		return []string{defaultALPN}
	}
	return out
}

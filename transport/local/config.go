package local

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

// AllocatorFactory 为每个 local endpoint 创建专属 ByteBuf 分配器。
type AllocatorFactory func(loop *transport.EventLoop) (buffer.Allocator, error)

type Config struct {
	WriteBufferWatermark transport.WriteBufferWatermark
	AllocatorFactory     AllocatorFactory
}

func DefaultConfig() Config {
	return Config{WriteBufferWatermark: transport.DefaultWriteBufferWatermark()}
}

func normalizeConfig(cfg Config) Config {
	cfg.WriteBufferWatermark = transport.NormalizeWriteBufferWatermark(cfg.WriteBufferWatermark)
	return cfg
}

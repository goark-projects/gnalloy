package udp

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

const defaultReadBufferSize = 2048

// AllocatorFactory 为每个 UDP endpoint 创建专属 ByteBuf 分配器。
type AllocatorFactory func(loop *transport.EventLoop) (buffer.Allocator, error)

type Config struct {
	ReuseAddr      bool
	ReusePort      bool
	ReadBufferSize int

	AllocatorFactory AllocatorFactory
}

func DefaultConfig() Config {
	return Config{ReuseAddr: true, ReadBufferSize: defaultReadBufferSize}
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if !cfg.ReuseAddr {
		cfg.ReuseAddr = def.ReuseAddr
	}
	if cfg.ReadBufferSize <= 0 {
		cfg.ReadBufferSize = def.ReadBufferSize
	}
	return cfg
}

type socketOptions struct {
	reuseAddr      bool
	reusePort      bool
	readBufferSize int
}

func (c Config) socketOptions() socketOptions {
	return socketOptions{
		reuseAddr:      c.ReuseAddr,
		reusePort:      c.ReusePort,
		readBufferSize: c.ReadBufferSize,
	}
}

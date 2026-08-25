package tcp

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

const (
	defaultBacklog        = 1024
	defaultReadBufferSize = 4096
)

// AllocatorFactory 为 Worker EventLoop 创建专属 ByteBuf 分配器。
type AllocatorFactory func(loop *transport.EventLoop) (buffer.Allocator, error)

type Config struct {
	Backlog        int
	ReuseAddr      bool
	ReusePort      bool
	NoDelay        bool
	ReadBufferSize int

	AllocatorFactory AllocatorFactory

	// IOUringFixedBuffers 将 allocator 暴露的稳定内存块注册到 io_uring。
	// 该开关仅适用于 Linux io_uring + 支持 FixedBufferProvider 的 allocator。
	IOUringFixedBuffers bool
}

func DefaultConfig() Config {
	return Config{
		Backlog:        defaultBacklog,
		ReuseAddr:      true,
		NoDelay:        true,
		ReadBufferSize: defaultReadBufferSize,
	}
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.Backlog <= 0 {
		cfg.Backlog = def.Backlog
	}
	if cfg.ReadBufferSize <= 0 {
		cfg.ReadBufferSize = def.ReadBufferSize
	}
	if !cfg.ReuseAddr {
		cfg.ReuseAddr = def.ReuseAddr
	}
	if !cfg.NoDelay {
		cfg.NoDelay = def.NoDelay
	}
	return cfg
}

type socketOptions struct {
	backlog        int
	reuseAddr      bool
	reusePort      bool
	noDelay        bool
	readBufferSize int
	iouringFixed   bool
}

func (c Config) socketOptions() socketOptions {
	return socketOptions{
		backlog:        c.Backlog,
		reuseAddr:      c.ReuseAddr,
		reusePort:      c.ReusePort,
		noDelay:        c.NoDelay,
		readBufferSize: c.ReadBufferSize,
		iouringFixed:   c.IOUringFixedBuffers,
	}
}

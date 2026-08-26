package unix

import (
	"os"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

const (
	defaultBacklog        = 1024
	defaultReadBufferSize = 4096
)

// AllocatorFactory 为 Worker EventLoop 创建专属 ByteBuf 分配器。
type AllocatorFactory func(loop *transport.EventLoop) (buffer.Allocator, error)

// Config 描述 Unix domain socket 的 Channel 默认参数。
type Config struct {
	// Backlog 控制监听 socket 的 accept 队列长度。
	Backlog int
	// RemoveStaleSocket 表示 bind 前删除同路径旧 socket 文件；默认开启。
	RemoveStaleSocket bool
	// FileMode 控制普通 socket 文件权限，0 表示不显式 chmod。
	FileMode os.FileMode
	// ReadBufferSize 控制 Channel 单次底层读缓冲区大小。
	ReadBufferSize int
	// WriteBufferWatermark 控制 Channel 出站缓冲区反压水位线。
	WriteBufferWatermark transport.WriteBufferWatermark
	// AllocatorFactory 为每个 EventLoop 提供独立 ByteBuf 分配器。
	AllocatorFactory AllocatorFactory
	// IOUringFixedBuffers 将 allocator 暴露的稳定内存块注册到 io_uring。
	IOUringFixedBuffers bool
}

func DefaultConfig() Config {
	return Config{
		Backlog:           defaultBacklog,
		RemoveStaleSocket: true,
		ReadBufferSize:    defaultReadBufferSize,
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
	if !cfg.RemoveStaleSocket {
		cfg.RemoveStaleSocket = def.RemoveStaleSocket
	}
	cfg.WriteBufferWatermark = transport.NormalizeWriteBufferWatermark(cfg.WriteBufferWatermark)
	return cfg
}

type socketOptions struct {
	backlog              int
	removeStaleSocket    bool
	fileMode             os.FileMode
	readBufferSize       int
	writeBufferWatermark transport.WriteBufferWatermark
	iouringFixed         bool
}

func (c Config) socketOptions() socketOptions {
	return socketOptions{
		backlog:              c.Backlog,
		removeStaleSocket:    c.RemoveStaleSocket,
		fileMode:             c.FileMode,
		readBufferSize:       c.ReadBufferSize,
		writeBufferWatermark: c.WriteBufferWatermark,
		iouringFixed:         c.IOUringFixedBuffers,
	}
}

func (o socketOptions) withListenOptions(options *channel.ChannelOptions) socketOptions {
	if v, ok := channel.OptionSoBacklog.GetIfSet(options); ok && v > 0 {
		o.backlog = v
	}
	return o
}

func (o socketOptions) withChildOptions(options *channel.ChannelOptions) socketOptions {
	if v, ok := channel.OptionReadBufferSize.GetIfSet(options); ok && v > 0 {
		o.readBufferSize = v
	}
	if v, ok := channel.OptionWriteBufferWatermark.GetIfSet(options); ok {
		o.writeBufferWatermark = transport.NormalizeWriteBufferWatermark(v)
	}
	return o
}

func (o socketOptions) withClientOptions(options *channel.ChannelOptions) socketOptions {
	return o.withChildOptions(options)
}

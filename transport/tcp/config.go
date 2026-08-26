package tcp

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

const (
	defaultBacklog              = 1024
	defaultReadBufferSize       = 4096
	defaultConnectTimeoutMillis = 30000
)

// AllocatorFactory 为 Worker EventLoop 创建专属 ByteBuf 分配器。
type AllocatorFactory func(loop *transport.EventLoop) (buffer.Allocator, error)

// Config 描述 TCP Transport 的底层 socket 和 Channel 默认参数。
type Config struct {
	// Backlog 控制监听 socket 的 accept 队列长度。
	Backlog int
	// ReuseAddr 控制监听 socket 的 SO_REUSEADDR，零值沿用默认开启。
	ReuseAddr bool
	// ReusePort 控制是否按 Boss EventLoop 数量创建 SO_REUSEPORT 监听 socket。
	ReusePort bool
	// NoDelay 控制连接 socket 的 TCP_NODELAY，零值沿用默认开启。
	NoDelay bool
	// KeepAlive 控制连接 socket 的 SO_KEEPALIVE。
	KeepAlive bool
	// SendBufferSize 控制 socket 发送缓冲区大小，0 表示使用系统默认值。
	SendBufferSize int
	// ReceiveBufferSize 控制 socket 接收缓冲区大小，0 表示使用系统默认值。
	ReceiveBufferSize int
	// SoLinger 控制连接 socket 的 SO_LINGER，-1 表示禁用。
	SoLinger int
	// ConnectTimeoutMillis 控制客户端 connect 超时，0 表示使用默认 30 秒。
	ConnectTimeoutMillis int
	// ReadBufferSize 控制 Channel 单次底层读缓冲区大小。
	ReadBufferSize int
	// WriteBufferWatermark 控制 Channel 出站缓冲区反压水位线。
	WriteBufferWatermark transport.WriteBufferWatermark

	// AllocatorFactory 为每个 EventLoop 提供独立 ByteBuf 分配器。
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
		SoLinger:       -1,

		ConnectTimeoutMillis: defaultConnectTimeoutMillis,
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
	if cfg.ConnectTimeoutMillis == 0 {
		cfg.ConnectTimeoutMillis = def.ConnectTimeoutMillis
	}
	if cfg.ConnectTimeoutMillis < 0 {
		cfg.ConnectTimeoutMillis = 0
	}
	if cfg.SoLinger == 0 {
		cfg.SoLinger = def.SoLinger
	}
	cfg.WriteBufferWatermark = transport.NormalizeWriteBufferWatermark(cfg.WriteBufferWatermark)
	if !cfg.ReuseAddr {
		cfg.ReuseAddr = def.ReuseAddr
	}
	if !cfg.NoDelay {
		cfg.NoDelay = def.NoDelay
	}
	return cfg
}

type socketOptions struct {
	backlog              int
	reuseAddr            bool
	reusePort            bool
	noDelay              bool
	keepAlive            bool
	sendBufferSize       int
	receiveBufferSize    int
	soLinger             int
	connectTimeoutMillis int
	readBufferSize       int
	writeBufferWatermark transport.WriteBufferWatermark
	iouringFixed         bool
}

func (c Config) socketOptions() socketOptions {
	return socketOptions{
		backlog:              c.Backlog,
		reuseAddr:            c.ReuseAddr,
		reusePort:            c.ReusePort,
		noDelay:              c.NoDelay,
		keepAlive:            c.KeepAlive,
		sendBufferSize:       c.SendBufferSize,
		receiveBufferSize:    c.ReceiveBufferSize,
		soLinger:             c.SoLinger,
		connectTimeoutMillis: c.ConnectTimeoutMillis,
		readBufferSize:       c.ReadBufferSize,
		writeBufferWatermark: c.WriteBufferWatermark,
		iouringFixed:         c.IOUringFixedBuffers,
	}
}

func (o socketOptions) withListenOptions(options *channel.ChannelOptions) socketOptions {
	if v, ok := channel.OptionSoBacklog.GetIfSet(options); ok && v > 0 {
		o.backlog = v
	}
	if v, ok := channel.OptionSoReuseAddr.GetIfSet(options); ok {
		o.reuseAddr = v
	}
	if v, ok := channel.OptionSoReusePort.GetIfSet(options); ok {
		o.reusePort = v
	}
	if v, ok := channel.OptionSoSndBuf.GetIfSet(options); ok && v > 0 {
		o.sendBufferSize = v
	}
	if v, ok := channel.OptionSoRcvBuf.GetIfSet(options); ok && v > 0 {
		o.receiveBufferSize = v
	}
	return o
}

func (o socketOptions) withChildOptions(options *channel.ChannelOptions) socketOptions {
	if v, ok := channel.OptionTcpNoDelay.GetIfSet(options); ok {
		o.noDelay = v
	}
	if v, ok := channel.OptionSoKeepAlive.GetIfSet(options); ok {
		o.keepAlive = v
	}
	if v, ok := channel.OptionSoSndBuf.GetIfSet(options); ok && v > 0 {
		o.sendBufferSize = v
	}
	if v, ok := channel.OptionSoRcvBuf.GetIfSet(options); ok && v > 0 {
		o.receiveBufferSize = v
	}
	if v, ok := channel.OptionSoLinger.GetIfSet(options); ok {
		o.soLinger = v
	}
	if v, ok := channel.OptionReadBufferSize.GetIfSet(options); ok && v > 0 {
		o.readBufferSize = v
	}
	if v, ok := channel.OptionWriteBufferWatermark.GetIfSet(options); ok {
		o.writeBufferWatermark = transport.NormalizeWriteBufferWatermark(v)
	}
	return o
}

func (o socketOptions) withClientOptions(options *channel.ChannelOptions) socketOptions {
	o = o.withChildOptions(options)
	if v, ok := channel.OptionConnectTimeoutMillis.GetIfSet(options); ok {
		if v < 0 {
			v = 0
		}
		o.connectTimeoutMillis = v
	}
	return o
}

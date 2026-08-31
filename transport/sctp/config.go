package sctp

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

const (
	defaultBacklog              = 1024
	defaultReadBufferSize       = 4096
	defaultConnectTimeoutMillis = 30000
	defaultInboundStreams       = 16
	defaultOutboundStreams      = 16
)

// AllocatorFactory 为 Worker EventLoop 创建专属 ByteBuf 分配器。
type AllocatorFactory func(loop *transport.EventLoop) (buffer.Allocator, error)

// Config 描述 SCTP one-to-one stream transport 的 socket 参数。
type Config struct {
	Backlog              int
	ReuseAddr            bool
	KeepAlive            bool
	SendBufferSize       int
	ReceiveBufferSize    int
	SoLinger             int
	ConnectTimeoutMillis int
	ReadBufferSize       int
	InboundStreams       uint16
	OutboundStreams      uint16
	WriteBufferWatermark transport.WriteBufferWatermark

	AllocatorFactory AllocatorFactory
}

func DefaultConfig() Config {
	return Config{
		Backlog:              defaultBacklog,
		ReuseAddr:            true,
		ReadBufferSize:       defaultReadBufferSize,
		SoLinger:             -1,
		ConnectTimeoutMillis: defaultConnectTimeoutMillis,
		InboundStreams:       defaultInboundStreams,
		OutboundStreams:      defaultOutboundStreams,
	}
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.Backlog <= 0 {
		cfg.Backlog = def.Backlog
	}
	if !cfg.ReuseAddr {
		cfg.ReuseAddr = def.ReuseAddr
	}
	if cfg.ReadBufferSize <= 0 {
		cfg.ReadBufferSize = def.ReadBufferSize
	}
	if cfg.SoLinger == 0 {
		cfg.SoLinger = def.SoLinger
	}
	if cfg.ConnectTimeoutMillis == 0 {
		cfg.ConnectTimeoutMillis = def.ConnectTimeoutMillis
	}
	if cfg.ConnectTimeoutMillis < 0 {
		cfg.ConnectTimeoutMillis = 0
	}
	if cfg.InboundStreams == 0 {
		cfg.InboundStreams = def.InboundStreams
	}
	if cfg.OutboundStreams == 0 {
		cfg.OutboundStreams = def.OutboundStreams
	}
	cfg.WriteBufferWatermark = transport.NormalizeWriteBufferWatermark(cfg.WriteBufferWatermark)
	return cfg
}

type socketOptions struct {
	backlog              int
	reuseAddr            bool
	keepAlive            bool
	sendBufferSize       int
	receiveBufferSize    int
	soLinger             int
	connectTimeoutMillis int
	readBufferSize       int
	inboundStreams       uint16
	outboundStreams      uint16
	writeBufferWatermark transport.WriteBufferWatermark
}

func (c Config) socketOptions() socketOptions {
	return socketOptions{
		backlog:              c.Backlog,
		reuseAddr:            c.ReuseAddr,
		keepAlive:            c.KeepAlive,
		sendBufferSize:       c.SendBufferSize,
		receiveBufferSize:    c.ReceiveBufferSize,
		soLinger:             c.SoLinger,
		connectTimeoutMillis: c.ConnectTimeoutMillis,
		readBufferSize:       c.ReadBufferSize,
		inboundStreams:       c.InboundStreams,
		outboundStreams:      c.OutboundStreams,
		writeBufferWatermark: c.WriteBufferWatermark,
	}
}

func (o socketOptions) withListenOptions(options *channel.ChannelOptions) socketOptions {
	if v, ok := channel.OptionSoBacklog.GetIfSet(options); ok && v > 0 {
		o.backlog = v
	}
	if v, ok := channel.OptionSoReuseAddr.GetIfSet(options); ok {
		o.reuseAddr = v
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

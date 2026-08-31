package http2

import "goark.dev/gnalloy/channel"

// ConnectionControllerConfig 描述 HTTP/2 连接级协议控制边界。
type ConnectionControllerConfig struct {
	// Server 表示本端是服务端；用于校验本端和对端 stream ID 奇偶性。
	Server bool
	// InitialConnectionWindow 是本端接收连接窗口，0 使用 RFC 默认值。
	InitialConnectionWindow int32
	// InitialStreamWindow 是本端接收 stream 窗口，0 使用 RFC 默认值。
	InitialStreamWindow int32
	// MaxConcurrentStreams 限制对端可同时打开的 stream 数，0 表示不额外限制。
	MaxConcurrentStreams int
	// DisablePush 禁用本端接收 server push。
	DisablePush bool
}

// ConnectionController 维护 HTTP/2 SETTINGS、GOAWAY、并发 stream 和入站流控。
type ConnectionController struct {
	cfg                     ConnectionControllerConfig
	localSettings           SettingsSnapshot
	remoteSettings          SettingsSnapshot
	connectionReceiveWindow int32
	initialStreamWindow     int32
	streams                 map[StreamID]*multiplexedStream
	goAwayReceived          bool
	goAwayLastStream        StreamID
}

// NewConnectionController 创建连接级协议控制 handler。
func NewConnectionController(cfg ConnectionControllerConfig) (*ConnectionController, error) {
	if cfg.InitialConnectionWindow < 0 || cfg.InitialStreamWindow < 0 || cfg.MaxConcurrentStreams < 0 {
		return nil, ErrFlowControl
	}
	connWindow := normalizedWindow(cfg.InitialConnectionWindow)
	streamWindow := normalizedWindow(cfg.InitialStreamWindow)
	return &ConnectionController{
		cfg:                     cfg,
		localSettings:           defaultSettingsSnapshot(streamWindow, cfg.MaxConcurrentStreams, !cfg.DisablePush),
		remoteSettings:          defaultSettingsSnapshot(defaultInitialWindowSize, 0, true),
		connectionReceiveWindow: connWindow,
		initialStreamWindow:     streamWindow,
		streams:                 make(map[StreamID]*multiplexedStream, 16),
	}, nil
}

// LocalSettings 返回本端设置快照。
func (c *ConnectionController) LocalSettings() SettingsSnapshot {
	return c.localSettings
}

// RemoteSettings 返回对端已应用设置快照。
func (c *ConnectionController) RemoteSettings() SettingsSnapshot {
	return c.remoteSettings
}

// ConnectionReceiveWindow 返回当前连接级接收窗口。
func (c *ConnectionController) ConnectionReceiveWindow() int32 {
	return c.connectionReceiveWindow
}

// StreamReceiveWindow 返回指定 stream 接收窗口。
func (c *ConnectionController) StreamReceiveWindow(id StreamID) int32 {
	if stream := c.streams[id]; stream != nil {
		return stream.recvWindow
	}
	return c.initialStreamWindow
}

// ActiveStreams 返回连接控制器仍跟踪的 stream 数。
func (c *ConnectionController) ActiveStreams() int {
	return len(c.streams)
}

// ChannelRead 校验并应用入站连接级语义后继续传播 frame。
func (c *ConnectionController) ChannelRead(ctx *channel.HandlerContext, msg any) {
	frame, ok := msg.(TypedFrame)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := c.readFrame(frame); err != nil {
		frame.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(msg)
}

// Write 校验并应用出站连接级语义。
func (c *ConnectionController) Write(ctx *channel.HandlerContext, msg any) error {
	frame, ok := msg.(TypedFrame)
	if !ok {
		return ctx.Write(msg)
	}
	if err := c.writeFrame(frame); err != nil {
		frame.Release()
		return err
	}
	return ctx.Write(msg)
}

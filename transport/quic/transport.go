package quic

import (
	"context"
	"sync/atomic"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

// Transport 将 RFC9000 QUIC 连接栈接入 gnalloy Bootstrap/Dialer。
//
// 服务端 child channel 和客户端 Dialer 返回值都表示一条 QUIC 双向 stream，
// 上层业务只处理 ByteBuf 和 Pipeline 事件，不直接依赖 quic-go 运行时类型。
type Transport struct {
	cfg    Config
	nextID atomic.Uint64
}

// NewTransport 创建 RFC9000 QUIC stream transport。
func NewTransport(cfg Config) *Transport {
	return &Transport{cfg: cfg}
}

// Bind 创建 QUIC listener，并把每条入站双向 stream 装配成 gnalloy Channel。
func (t *Transport) Bind(ctx context.Context, cfg bootstrap.ServerConfig) (bootstrap.Server, error) {
	if cfg.WorkerGroup == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if cfg.ChildInitializer == nil {
		return nil, bootstrap.ErrMissingChildHandler
	}
	listener, err := ListenAddr(cfg.Address, t.cfg)
	if err != nil {
		return nil, err
	}
	serverCtx, cancel := context.WithCancel(context.Background())
	server := newServer(serverConfig{
		listener:  listener,
		cfg:       cfg,
		transport: t,
		ctx:       serverCtx,
		cancel:    cancel,
	})
	server.start()
	return server, nil
}

// Dial 连接远端 QUIC 服务端，打开一条双向 stream，并返回对应 Channel。
func (t *Transport) Dial(ctx context.Context, cfg bootstrap.ClientConfig) (channel.Channel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.Group == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if cfg.Initializer == nil {
		cfg.Initializer = func(channel.Channel) error { return nil }
	}
	conn, err := t.dialConnection(ctx, cfg.Address)
	if err != nil {
		return nil, err
	}
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		_ = conn.CloseWithError(0, "open stream failed")
		return nil, err
	}
	loop, err := cfg.Group.Next()
	if err != nil {
		stream.CancelRead(0)
		stream.CancelWrite(0)
		_ = conn.CloseWithError(0, "event loop unavailable")
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(context.Background())
	endpoint, err := newStreamEndpoint(streamEndpointConfig{
		id:     t.nextChannelID(),
		stream: stream,
		timer:  loop.Timer(),
		onClose: func() {
			cancel()
			_ = conn.CloseWithError(0, "stream channel closed")
		},
	})
	if err != nil {
		cancel()
		_ = conn.CloseWithError(0, "stream channel init failed")
		return nil, err
	}
	cfg.Apply(endpoint.Channel())
	endpoint.applyOptions()
	if err := cfg.Initializer(endpoint.Channel()); err != nil {
		_ = endpoint.Close()
		return nil, err
	}
	endpoint.activate(streamCtx)
	return endpoint.Channel(), nil
}

func (t *Transport) dialConnection(ctx context.Context, address string) (Connection, error) {
	if t.cfg.Enable0RTT {
		return DialAddrEarly(ctx, address, t.cfg)
	}
	return DialAddr(ctx, address, t.cfg)
}

func (t *Transport) nextChannelID() transport.ChannelID {
	return transport.ChannelID(t.nextID.Add(1))
}

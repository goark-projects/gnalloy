package l2

import (
	"context"
	"sync/atomic"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

// Transport 将 L2 Driver 接入 gnalloy Bootstrap/Dialer。
type Transport struct {
	cfg    Config
	nextID atomic.Uint64
}

// NewTransport 创建 L2 transport。
func NewTransport(cfg Config) *Transport {
	return &Transport{cfg: cfg}
}

// Bind 打开一个二层接口，并把接口收发面暴露为单个 child Channel。
func (t *Transport) Bind(ctx context.Context, cfg bootstrap.ServerConfig) (bootstrap.Server, error) {
	if cfg.WorkerGroup == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if cfg.ChildInitializer == nil {
		return nil, bootstrap.ErrMissingChildHandler
	}
	ep, err := t.newChannelEndpoint(ctx, channelEndpointConfig{
		address: cfg.Address,
		group:   cfg.WorkerGroup,
		apply:   cfg.ApplyChild,
		init:    cfg.ChildInitializer,
	})
	if err != nil {
		return nil, err
	}
	return &Server{endpoint: ep}, nil
}

// Dial 打开一个二层接口，并返回对应 Channel。
func (t *Transport) Dial(ctx context.Context, cfg bootstrap.ClientConfig) (channel.Channel, error) {
	if cfg.Group == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if cfg.Initializer == nil {
		cfg.Initializer = func(channel.Channel) error { return nil }
	}
	ep, err := t.newChannelEndpoint(ctx, channelEndpointConfig{
		address: cfg.Address,
		group:   cfg.Group,
		apply:   cfg.Apply,
		init:    cfg.Initializer,
	})
	if err != nil {
		return nil, err
	}
	return ep.Channel(), nil
}

type channelEndpointConfig struct {
	address string
	group   *transport.EventLoopGroup
	apply   func(channel.Channel)
	init    func(channel.Channel) error
}

func (t *Transport) newChannelEndpoint(ctx context.Context, cfg channelEndpointConfig) (*channelEndpoint, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	l2cfg := normalizeConfig(t.cfg, cfg.address)
	driver := l2cfg.Driver
	if driver == nil {
		driver = nativeDriver{}
	}
	native, err := driver.Open(ctx, l2cfg)
	if err != nil {
		return nil, err
	}
	loop, err := cfg.group.Next()
	if err != nil {
		_ = native.Close()
		return nil, err
	}
	ep, err := newChannelEndpoint(channelEndpointOptions{
		id:     transport.ChannelID(t.nextID.Add(1)),
		native: native,
		cfg:    l2cfg,
		timer:  loop.Timer(),
	})
	if err != nil {
		_ = native.Close()
		return nil, err
	}
	if cfg.apply != nil {
		cfg.apply(ep.Channel())
	}
	ep.applyOptions()
	if cfg.init != nil {
		if err := cfg.init(ep.Channel()); err != nil {
			_ = ep.Close()
			return nil, err
		}
	}
	ep.activate()
	return ep, nil
}

// Server 是 L2 Bind 后返回的服务端句柄。
type Server struct {
	endpoint *channelEndpoint
}

func (s *Server) Addr() string {
	if s == nil || s.endpoint == nil {
		return ""
	}
	return s.endpoint.Addr()
}

func (s *Server) Close() error {
	if s == nil || s.endpoint == nil {
		return nil
	}
	return s.endpoint.Close()
}

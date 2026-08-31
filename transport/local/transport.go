package local

import (
	"context"
	"sync/atomic"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

var nextLocalChannelID atomic.Uint64

// Transport 把 in-VM local channel pair 接入 Bootstrap/Dialer。
type Transport struct {
	cfg Config
}

func NewTransport(cfg Config) *Transport {
	return &Transport{cfg: normalizeConfig(cfg)}
}

func (t *Transport) Bind(_ context.Context, cfg bootstrap.ServerConfig) (bootstrap.Server, error) {
	if cfg.BossGroup == nil || cfg.WorkerGroup == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if cfg.ChildInitializer == nil {
		return nil, bootstrap.ErrMissingChildHandler
	}
	server := newServer(cfg.Address, t.cfg, cfg)
	if err := registerServer(cfg.Address, server); err != nil {
		return nil, err
	}
	return server, nil
}

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
	server, ok := lookupServer(cfg.Address)
	if !ok || server.isClosed() {
		return nil, ErrServerNotFound
	}
	client, err := t.newEndpoint(ctx, cfg.Group, t.cfg, cfg.Apply, cfg.Initializer)
	if err != nil {
		return nil, err
	}
	child, err := server.accept(ctx)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	client.connect(child)
	child.connect(client)
	if err := child.activate(ctx); err != nil {
		_ = client.Close()
		_ = child.Close()
		return nil, err
	}
	if err := client.activate(ctx); err != nil {
		_ = client.Close()
		_ = child.Close()
		return nil, err
	}
	return client.Channel(), nil
}

func (t *Transport) newEndpoint(ctx context.Context, group *transport.EventLoopGroup, cfg Config, apply func(channel.Channel), init func(channel.Channel) error) (*endpoint, error) {
	loop, err := group.Next()
	if err != nil {
		return nil, err
	}
	alloc, err := newAllocator(cfg, loop)
	if err != nil {
		return nil, err
	}
	ep := newEndpoint(localEndpointConfig{
		id:        transport.ChannelID(nextLocalChannelID.Add(1)),
		loop:      loop,
		allocator: alloc,
		watermark: cfg.WriteBufferWatermark,
	})
	if apply != nil {
		apply(ep.Channel())
	}
	ep.applyOptions()
	if init != nil {
		if err := init(ep.Channel()); err != nil {
			_ = ep.Close()
			return nil, err
		}
	}
	return ep, nil
}

func newAllocator(cfg Config, loop *transport.EventLoop) (buffer.Allocator, error) {
	if cfg.AllocatorFactory != nil {
		return cfg.AllocatorFactory(loop)
	}
	return buffer.NewHeapAllocator(), nil
}

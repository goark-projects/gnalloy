package bootstrap

import (
	"context"
	"errors"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

// ChildInitializer 初始化新建 Channel 的 Pipeline。
type ChildInitializer func(ch channel.Channel) error

// Server 是 Bind 后返回的服务端句柄。
type Server interface {
	Addr() string
	Close() error
}

// ServerTransport 是具体传输实现需要满足的服务端绑定契约。
type ServerTransport interface {
	Bind(ctx context.Context, cfg ServerConfig) (Server, error)
}

type ServerConfig struct {
	Address string

	BossGroup   *transport.EventLoopGroup
	WorkerGroup *transport.EventLoopGroup

	ChildInitializer ChildInitializer
}

type ServerBootstrap struct {
	bossGroup   *transport.EventLoopGroup
	workerGroup *transport.EventLoopGroup

	childInitializer ChildInitializer
	serverTransport  ServerTransport
}

func NewServerBootstrap() *ServerBootstrap {
	return &ServerBootstrap{}
}

func (b *ServerBootstrap) Group(bossGroup *transport.EventLoopGroup, workerGroup *transport.EventLoopGroup) *ServerBootstrap {
	b.bossGroup = bossGroup
	b.workerGroup = workerGroup
	return b
}

func (b *ServerBootstrap) ChildHandler(handler func(ch channel.Channel)) *ServerBootstrap {
	if handler == nil {
		b.childInitializer = nil
		return b
	}
	b.childInitializer = func(ch channel.Channel) error {
		handler(ch)
		return nil
	}
	return b
}

func (b *ServerBootstrap) ChildInitializer(initializer ChildInitializer) *ServerBootstrap {
	b.childInitializer = initializer
	return b
}

func (b *ServerBootstrap) Transport(serverTransport ServerTransport) *ServerBootstrap {
	b.serverTransport = serverTransport
	return b
}

func (b *ServerBootstrap) Bind(address string) (Server, error) {
	return b.BindContext(context.Background(), address)
}

func (b *ServerBootstrap) BindContext(ctx context.Context, address string) (Server, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := b.validate(address); err != nil {
		return nil, err
	}

	startedBoss, err := ensureGroupStarted(ctx, b.bossGroup)
	if err != nil {
		return nil, err
	}
	startedWorker := false
	if b.workerGroup != b.bossGroup {
		startedWorker, err = ensureGroupStarted(ctx, b.workerGroup)
		if err != nil {
			if startedBoss {
				_ = b.bossGroup.Shutdown(context.Background())
			}
			return nil, err
		}
	}

	server, err := b.serverTransport.Bind(ctx, ServerConfig{
		Address:          address,
		BossGroup:        b.bossGroup,
		WorkerGroup:      b.workerGroup,
		ChildInitializer: b.childInitializer,
	})
	if err != nil {
		if startedWorker {
			_ = b.workerGroup.Shutdown(context.Background())
		}
		if startedBoss {
			_ = b.bossGroup.Shutdown(context.Background())
		}
		return nil, err
	}
	return server, nil
}

func (b *ServerBootstrap) validate(address string) error {
	if address == "" {
		return ErrEmptyAddress
	}
	if b.bossGroup == nil || b.workerGroup == nil {
		return ErrMissingGroup
	}
	if b.childInitializer == nil {
		return ErrMissingChildHandler
	}
	if b.serverTransport == nil {
		return ErrMissingTransport
	}
	return nil
}

func ensureGroupStarted(ctx context.Context, group *transport.EventLoopGroup) (bool, error) {
	if group == nil {
		return false, ErrMissingGroup
	}
	if group.IsRunning() {
		return false, nil
	}
	err := group.Start(ctx)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, transport.ErrEventLoopGroupRunning) {
		return false, nil
	}
	return false, err
}

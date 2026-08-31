package sctp

import (
	"context"
	"sync/atomic"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

type listenSocket struct {
	fd     transport.FDRef
	addr   string
	family int
}

// Transport 把 Bootstrap 接到 SCTP one-to-one stream socket。
type Transport struct {
	cfg    Config
	nextID atomic.Uint64
}

func NewTransport(cfg Config) *Transport {
	return &Transport{cfg: normalizeConfig(cfg)}
}

func (t *Transport) Bind(ctx context.Context, cfg bootstrap.ServerConfig) (bootstrap.Server, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.BossGroup == nil || cfg.WorkerGroup == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if cfg.ChildInitializer == nil {
		return nil, bootstrap.ErrMissingChildHandler
	}
	if err := ValidateRuntime(RuntimeCheck{
		Role:        EndpointRoleServer,
		Address:     cfg.Address,
		Config:      t.cfg,
		BossGroup:   cfg.BossGroup,
		WorkerGroup: cfg.WorkerGroup,
	}); err != nil {
		return nil, err
	}
	base := t.cfg.socketOptions()
	listenOptions := base.withListenOptions(cfg.Options)
	childOptions := base.withChildOptions(cfg.ChildOptions)
	ls, err := listenSCTP(cfg.Address, listenOptions)
	if err != nil {
		return nil, err
	}
	server := &Server{
		addr:              ls.addr,
		options:           childOptions,
		serverConfig:      cfg,
		workerGroup:       cfg.WorkerGroup,
		childInitializer:  cfg.ChildInitializer,
		allocatorFactory:  t.cfg.AllocatorFactory,
		allocators:        make(map[transport.EventLoopID]buffer.Allocator, cfg.WorkerGroup.Size()),
		active:            make(map[transport.ChannelID]activeChild, 128),
		transportIDSource: t,
	}
	server.acceptor = &acceptor{id: t.nextChannelID(), server: server, fd: ls.fd}
	loop, err := cfg.BossGroup.RegisterNext(ctx, server.acceptor, transport.ReadyRead, func(loop *transport.EventLoop, handler transport.EventHandler) error {
		handler.(*acceptor).loop = loop
		return nil
	})
	if err != nil {
		_ = server.Close()
		return nil, err
	}
	server.bossLoop = loop
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
	if err := ValidateRuntime(RuntimeCheck{
		Role:    EndpointRoleClient,
		Address: cfg.Address,
		Config:  t.cfg,
		Group:   cfg.Group,
	}); err != nil {
		return nil, err
	}
	opts := t.cfg.socketOptions().withClientOptions(cfg.Options)
	fd, err := dialSCTP(cfg.Address, opts)
	if err != nil {
		return nil, err
	}
	loop, err := cfg.Group.Next()
	if err != nil {
		_ = closeFD(fd)
		return nil, err
	}
	alloc, err := t.clientAllocator(loop)
	if err != nil {
		_ = closeFD(fd)
		return nil, err
	}
	unsafeCfg := channel.UnsafeConfig{
		ID:                   t.nextChannelID(),
		FD:                   fd,
		Allocator:            alloc,
		Poller:               loop.Poller(),
		ReadWriter:           newNativeReadWriter(),
		ReadBufferSize:       opts.readBufferSize,
		WriteBufferWatermark: opts.writeBufferWatermark,
		Timer:                loop.Timer(),
	}
	var unsafeCh *channel.Unsafe
	unsafeCfg.CloseHook = func(ch *channel.Unsafe) {
		_ = loop.Deregister(ch.ID())
		_ = alloc.Close()
	}
	ch, unsafeCh, err := cfg.NewChannel(unsafeCfg)
	if err != nil {
		_ = closeFD(fd)
		_ = alloc.Close()
		return nil, err
	}
	cfg.Apply(ch)
	if err := cfg.Initializer(ch); err != nil {
		_ = unsafeCh.Close()
		ch.Pipeline().FireExceptionCaught(err)
		return nil, err
	}
	if err := loop.Invoke(ctx, func() error {
		if err := loop.Register(unsafeCh, unsafeCh.InitialInterest()); err != nil {
			return err
		}
		return unsafeCh.Activate()
	}); err != nil {
		_ = unsafeCh.Close()
		ch.Pipeline().FireExceptionCaught(err)
		return nil, err
	}
	return ch, nil
}

func (t *Transport) nextChannelID() transport.ChannelID {
	return transport.ChannelID(t.nextID.Add(1))
}

func (t *Transport) clientAllocator(loop *transport.EventLoop) (buffer.Allocator, error) {
	if t.cfg.AllocatorFactory != nil {
		return t.cfg.AllocatorFactory(loop)
	}
	return buffer.NewHeapAllocator(), nil
}

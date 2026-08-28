package main

import (
	"context"
	"fmt"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/tcp"
)

const shutdownTimeout = 5 * time.Second

type echoServer struct {
	addr    string
	server  bootstrap.Server
	boss    *transport.EventLoopGroup
	workers *transport.EventLoopGroup
}

func startEchoServer(ctx context.Context, cfg config) (*echoServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	boss, workers, err := newGroups(cfg)
	if err != nil {
		return nil, err
	}
	server, err := bindEchoServer(ctx, cfg, boss, workers)
	if err != nil {
		shutdownGroups(boss, workers)
		return nil, err
	}
	return &echoServer{addr: server.Addr(), server: server, boss: boss, workers: workers}, nil
}

func newGroups(cfg config) (*transport.EventLoopGroup, *transport.EventLoopGroup, error) {
	pollerConfig := transport.Config{
		Backend:         cfg.Backend,
		MultishotAccept: cfg.IOUringMultishotAccept,
		SQPoll:          cfg.IOUringSQPoll,
	}
	boss, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         cfg.Boss,
		PollerConfig: pollerConfig,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create boss group: %w", err)
	}
	workers, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         cfg.Workers,
		PollerConfig: pollerConfig,
	})
	if err != nil {
		_ = boss.Close()
		return nil, nil, fmt.Errorf("create worker group: %w", err)
	}
	return boss, workers, nil
}

func bindEchoServer(ctx context.Context, cfg config, boss *transport.EventLoopGroup, workers *transport.EventLoopGroup) (bootstrap.Server, error) {
	tcpConfig := tcp.DefaultConfig()
	tcpConfig.ReadBufferSize = cfg.ReadBufferSize
	tcpConfig.ReusePort = cfg.ReusePort
	tcpConfig.IOUringFixedBuffers = cfg.IOUringFixedBuffers
	if cfg.Mmap {
		tcpConfig.AllocatorFactory = tcp.NewMmapAllocatorFactory(buffer.MmapAllocatorConfig{
			BlockSize: cfg.MmapBlockSize,
			Blocks:    cfg.MmapBlocks,
		}, false)
	}
	return bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(tcp.NewTransport(tcpConfig)).
		ChildInitializer(func(ch channel.Channel) error {
			return ch.Pipeline().AddLast("echo", echoHandler{})
		}).
		BindContext(ctx, cfg.Addr)
}

func (s *echoServer) stop() {
	if s == nil {
		return
	}
	if s.server != nil {
		_ = s.server.Close()
	}
	shutdownGroups(s.boss, s.workers)
}

func shutdownGroups(groups ...*transport.EventLoopGroup) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	for _, group := range groups {
		_ = group.Shutdown(ctx)
	}
}

type echoHandler struct{}

func (echoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := ctx.WriteAndFlush(buf); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (echoHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
	_ = ctx.Close()
}

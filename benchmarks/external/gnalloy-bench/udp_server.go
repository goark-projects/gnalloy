package main

import (
	"context"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/udp"
)

func startUDPEchoServer(ctx context.Context, cfg config) (*echoServer, error) {
	boss, workers, err := newGroups(cfg)
	if err != nil {
		return nil, err
	}
	server, err := bindUDPEchoServer(ctx, cfg, boss, workers)
	if err != nil {
		shutdownGroups(boss, workers)
		return nil, err
	}
	return &echoServer{addr: server.Addr(), server: server, boss: boss, workers: workers}, nil
}

func bindUDPEchoServer(ctx context.Context, cfg config, boss *transport.EventLoopGroup, workers *transport.EventLoopGroup) (bootstrap.Server, error) {
	udpConfig := udp.DefaultConfig()
	udpConfig.ReadBufferSize = cfg.ReadBufferSize
	udpConfig.ReusePort = cfg.ReusePort
	if cfg.Mmap {
		udpConfig.AllocatorFactory = udp.NewMmapAllocatorFactory(buffer.MmapAllocatorConfig{
			BlockSize: cfg.MmapBlockSize,
			Blocks:    cfg.MmapBlocks,
		}, false)
	}
	return bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(udp.NewTransport(udpConfig)).
		ChildInitializer(func(ch channel.Channel) error {
			return ch.Pipeline().AddLast("echo", udpEchoHandler{})
		}).
		BindContext(ctx, cfg.Addr)
}

type udpEchoHandler struct{}

func (udpEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	datagram, ok := msg.(udp.Datagram)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := ctx.WriteAndFlush(datagram); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (udpEchoHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
	_ = ctx.Close()
}

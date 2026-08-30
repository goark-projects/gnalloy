package main

import (
	"context"

	"goark.dev/gnalloy/benchmarks/external/internal/benchhttp"
	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/http1"
	gnalloytls "goark.dev/gnalloy/handler/tls"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/tcp"
)

func startHTTP1Server(ctx context.Context, cfg config) (*echoServer, error) {
	boss, workers, err := newGroups(cfg)
	if err != nil {
		return nil, err
	}
	server, err := bindHTTP1Server(ctx, cfg, boss, workers)
	if err != nil {
		shutdownGroups(boss, workers)
		return nil, err
	}
	return &echoServer{addr: server.Addr(), server: server, boss: boss, workers: workers}, nil
}

func bindHTTP1Server(ctx context.Context, cfg config, boss *transport.EventLoopGroup, workers *transport.EventLoopGroup) (bootstrap.Server, error) {
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
			if cfg.Protocol == "https1" {
				tlsConfig, err := serverTLSConfig(cfg)
				if err != nil {
					return err
				}
				if err := ch.Pipeline().AddLast("tls", gnalloytls.Server(gnalloytls.Config{TLS: tlsConfig})); err != nil {
					return err
				}
			}
			return addHTTP1Pipeline(ch, cfg)
		}).
		BindContext(ctx, cfg.Addr)
}

func addHTTP1Pipeline(ch channel.Channel, cfg config) error {
	decoder, err := http1.NewRequestDecoder(16*1024, 0)
	if err != nil {
		return err
	}
	encoder := http1.NewResponseEncoder()
	if cfg.Protocol == "https1" && cfg.Payload > 0 && cfg.Payload <= 16*1024 {
		encoder = http1.NewResponseEncoderWithOptions(http1.ResponseEncoderOptions{CoalesceBodyBytes: 16 * 1024})
	}
	if err := ch.Pipeline().AddLast("httpEncoder", encoder); err != nil {
		return err
	}
	if err := ch.Pipeline().AddLast("httpDecoder", decoder); err != nil {
		return err
	}
	if err := ch.Pipeline().AddLast("continue", http1.NewContinueHandler()); err != nil {
		return err
	}
	return ch.Pipeline().AddLast("handler", http1Handler{
		body: benchhttp.ResponseBody(cfg.Payload),
		headers: http1.Headers{
			"Content-Type": "application/octet-stream",
		},
	})
}

type http1Handler struct {
	body    []byte
	headers http1.Headers
}

func (h http1Handler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	req, ok := msg.(http1.Request)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	defer req.Release()
	resp := http1.Response{
		StatusCode: 200,
		Headers:    h.headers,
		Body:       buffer.NewSharedBuffer(h.body),
	}
	if err := ctx.Channel().WriteAndFlush(resp); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (http1Handler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
	_ = ctx.Close()
}

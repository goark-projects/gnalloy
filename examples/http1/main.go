package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/examples/internal/exampleconfig"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/tcp"
)

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	opts := exampleconfig.Register(fs, ":9010")
	_ = fs.Parse(os.Args[1:])
	if err := opts.Resolve(); err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	boss, workers, err := opts.NewGroups()
	if err != nil {
		fatal(err)
	}
	defer shutdown(boss)
	defer shutdown(workers)

	tcpConfig, err := opts.TCPConfig()
	if err != nil {
		fatal(err)
	}
	server, err := bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(tcp.NewTransport(tcpConfig)).
		ChildInitializer(func(ch channel.Channel) error {
			decoder, err := http1.NewRequestDecoder(16*1024, 1<<20)
			if err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("httpEncoder", http1.NewResponseEncoder()); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("httpDecoder", decoder); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("continue", http1.NewContinueHandler()); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("handler", httpHandler{})
		}).
		BindContext(ctx, opts.Addr)
	if err != nil {
		fatal(err)
	}
	defer server.Close()

	fmt.Printf("gnalloy http1 listening on %s backend=%s boss=%d workers=%d reuseport=%v mmap=%v\n",
		server.Addr(), opts.BackendLabel(), opts.Boss, opts.Workers, opts.ReusePort, opts.Mmap)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}

type httpHandler struct{}

func (httpHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	req, ok := msg.(http1.Request)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	defer req.Release()

	body, err := ctx.Channel().Allocator().Acquire(len("gnalloy http1\n"))
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if _, err := body.WriteBytes([]byte("gnalloy http1\n")); err != nil {
		body.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	resp := http1.Response{
		StatusCode: 200,
		Headers: http1.Headers{
			"Content-Type": "text/plain; charset=utf-8",
		},
		Body: body,
	}
	if !req.KeepAlive() {
		resp.Headers.Set("Connection", "close")
		ctx.Channel().WriteAndFlushFuture(resp).AddListener(func(channel.Future) {
			_ = ctx.Close()
		})
		return
	}
	if err := ctx.Channel().WriteAndFlush(resp); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (httpHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
	_ = ctx.Pipeline().Close()
}

func shutdown(group *transport.EventLoopGroup) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = group.Shutdown(ctx)
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

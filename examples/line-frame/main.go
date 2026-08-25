package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/examples/internal/exampleconfig"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/tcp"
)

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	opts := exampleconfig.Register(fs, ":9003")
	maxFrameLength := fs.Int("max-frame-length", 64*1024, "maximum line frame length")
	_ = fs.Parse(os.Args[1:])
	if err := opts.Resolve(); err != nil {
		fatal(err)
	}
	if *maxFrameLength <= 0 {
		fatal(codec.ErrInvalidFrameLength)
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
			decoder, err := codec.NewLineBasedFrameDecoder(*maxFrameLength)
			if err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("line", decoder); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("echo", lineEchoHandler{})
		}).
		BindContext(ctx, opts.Addr)
	if err != nil {
		fatal(err)
	}
	defer server.Close()

	fmt.Printf("gnalloy line-frame echo listening on %s backend=%s workers=%d reuseport=%v mmap=%v\n",
		server.Addr(), opts.BackendLabel(), opts.Workers, opts.ReusePort, opts.Mmap)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}

type lineEchoHandler struct{}

func (lineEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	frame, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	defer frame.Release()

	payload := frame.Bytes()
	out, err := ctx.Channel().Allocator().Acquire(len(payload) + 1)
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if _, err := out.WriteBytes(payload); err != nil {
		out.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if _, err := out.WriteBytes([]byte{'\n'}); err != nil {
		out.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Channel().WriteAndFlush(out); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (lineEchoHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
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

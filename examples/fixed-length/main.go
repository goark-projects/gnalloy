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
	opts := exampleconfig.Register(fs, ":9004")
	frameLength := fs.Int("frame-length", 4, "fixed frame length")
	_ = fs.Parse(os.Args[1:])
	if err := opts.Resolve(); err != nil {
		fatal(err)
	}
	if *frameLength <= 0 {
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
			decoder, err := codec.NewFixedLengthFrameDecoder(*frameLength)
			if err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("fixed", decoder); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("echo", fixedEchoHandler{})
		}).
		BindContext(ctx, opts.Addr)
	if err != nil {
		fatal(err)
	}
	defer server.Close()

	fmt.Printf("gnalloy fixed-length echo listening on %s backend=%s workers=%d reuseport=%v mmap=%v frameLength=%d\n",
		server.Addr(), opts.BackendLabel(), opts.Workers, opts.ReusePort, opts.Mmap, *frameLength)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}

type fixedEchoHandler struct{}

func (fixedEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	frame, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := ctx.Channel().WriteAndFlush(frame); err != nil {
		frame.Release()
		ctx.FireExceptionCaught(err)
	}
}

func (fixedEchoHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
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

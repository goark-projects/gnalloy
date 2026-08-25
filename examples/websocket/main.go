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
	"goark.dev/gnalloy/codec/websocket"
	"goark.dev/gnalloy/examples/internal/exampleconfig"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/tcp"
)

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	opts := exampleconfig.Register(fs, ":9011")
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
			httpDecoder, err := http1.NewRequestDecoder(16*1024, 0)
			if err != nil {
				return err
			}
			frameDecoder, err := websocket.NewFrameDecoder(1 << 20)
			if err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("httpEncoder", http1.NewResponseEncoder()); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("wsEncoder", websocket.NewFrameEncoder()); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("httpDecoder", httpDecoder); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("handshake", websocket.NewServerHandshakeHandler("/", "httpDecoder", "handshake")); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("wsDecoder", frameDecoder); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("fragments", websocket.NewFragmentAggregator(1<<20)); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("control", websocket.NewControlFrameHandler()); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("echo", websocketEchoHandler{})
		}).
		BindContext(ctx, opts.Addr)
	if err != nil {
		fatal(err)
	}
	defer server.Close()

	fmt.Printf("gnalloy websocket echo listening on %s backend=%s boss=%d workers=%d reuseport=%v mmap=%v\n",
		server.Addr(), opts.BackendLabel(), opts.Boss, opts.Workers, opts.ReusePort, opts.Mmap)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}

type websocketEchoHandler struct{}

func (websocketEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	frame, ok := msg.(websocket.Frame)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if frame.Opcode != websocket.OpcodeText && frame.Opcode != websocket.OpcodeBinary {
		if frame.Payload != nil {
			frame.Payload.Release()
		}
		return
	}
	if err := ctx.Channel().WriteAndFlush(websocket.Frame{Final: true, Opcode: frame.Opcode, Payload: frame.Payload}); err != nil {
		if frame.Payload != nil {
			frame.Payload.Release()
		}
		ctx.FireExceptionCaught(err)
	}
}

func (websocketEchoHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
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

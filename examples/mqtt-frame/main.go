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
	"goark.dev/gnalloy/codec/mqtt"
	"goark.dev/gnalloy/examples/internal/exampleconfig"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/tcp"
)

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	opts := exampleconfig.Register(fs, ":9014")
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
			frameDecoder, err := mqtt.NewFrameDecoder(1 << 20)
			if err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("prepender", mqtt.NewFramePrepender()); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("packetEncoder", mqtt.NewPacketEncoder()); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("frame", frameDecoder); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("typedFrame", mqtt.NewTypedFrameDecoder()); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("packetDecoder", mqtt.NewPacketDecoder()); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("handler", mqttFrameHandler{})
		}).
		BindContext(ctx, opts.Addr)
	if err != nil {
		fatal(err)
	}
	defer server.Close()

	fmt.Printf("gnalloy mqtt-frame listening on %s backend=%s boss=%d workers=%d reuseport=%v mmap=%v\n",
		server.Addr(), opts.BackendLabel(), opts.Boss, opts.Workers, opts.ReusePort, opts.Mmap)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}

type mqttFrameHandler struct{}

func (mqttFrameHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch packet := msg.(type) {
	case mqtt.ConnectPacket:
		if err := ctx.Channel().WriteAndFlush(mqtt.ConnAckPacket{}); err != nil {
			ctx.FireExceptionCaught(err)
		}
	case mqtt.PingReqPacket:
		if err := ctx.Channel().WriteAndFlush(mqtt.PingRespPacket{}); err != nil {
			ctx.FireExceptionCaught(err)
		}
	case mqtt.PublishPacket:
		if err := ctx.Channel().WriteAndFlush(packet); err != nil {
			packet.Release()
			ctx.FireExceptionCaught(err)
		}
	case mqtt.DisconnectPacket:
		_ = ctx.Close()
	default:
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
}

func (mqttFrameHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
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

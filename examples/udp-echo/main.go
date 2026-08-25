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
	"goark.dev/gnalloy/transport/udp"
)

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	opts := exampleconfig.Register(fs, ":9002")
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

	udpConfig, err := opts.UDPConfig()
	if err != nil {
		fatal(err)
	}
	server, err := bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(udp.NewTransport(udpConfig)).
		ChildInitializer(func(ch channel.Channel) error {
			if err := ch.Pipeline().AddLast("datagramDecoder", udp.NewDatagramToMessageDecoder(datagramPayloadDecoder{})); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("echo", datagramEchoHandler{}); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("datagramEncoder", udp.NewMessageToDatagramEncoderFunc(func(any) bool { return false }, nil))
		}).
		BindContext(ctx, opts.Addr)
	if err != nil {
		fatal(err)
	}
	defer server.Close()

	fmt.Printf("gnalloy udp echo listening on %s backend=%s workers=%d reuseport=%v mmap=%v\n",
		server.Addr(), opts.BackendLabel(), opts.Workers, opts.ReusePort, opts.Mmap)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}

type datagramPayloadDecoder struct{}

func (datagramPayloadDecoder) AcceptDatagram(udp.Datagram) bool {
	return true
}

func (datagramPayloadDecoder) DecodeDatagram(_ *channel.HandlerContext, payload buffer.ByteBuf, out *codec.MessageList) error {
	frame, err := payload.Slice(payload.ReaderIndex(), payload.ReadableBytes())
	if err != nil {
		return err
	}
	out.Add(frame)
	return nil
}

type datagramEchoHandler struct{}

func (datagramEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	addressed, ok := msg.(udp.Addressed)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	payload, ok := addressed.Message.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := ctx.Channel().WriteAndFlush(udp.Addressed{Message: payload, Addr: addressed.Addr}); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (datagramEchoHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
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

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
	"goark.dev/gnalloy/examples/internal/exampleconfig"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/quic"
	"goark.dev/gnalloy/transport/udp"
)

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	opts := exampleconfig.Register(fs, ":9003")
	shortCIDLen := fs.Int("short-cid-len", 0, "short header destination connection id length")
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
			if err := ch.Pipeline().AddLast("quicEncoder", quic.NewPacketToDatagramEncoder()); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("quicDecoder", quic.NewDatagramToPacketDecoder(quic.DatagramToPacketDecoderConfig{
				HeaderParseOptions: quic.HeaderParseOptions{ShortDestinationIDLength: *shortCIDLen},
			})); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("dump", quicPacketDumpHandler{})
		}).
		BindContext(ctx, opts.Addr)
	if err != nil {
		fatal(err)
	}
	defer server.Close()

	fmt.Printf("gnalloy quic packet listener on %s backend=%s\n", server.Addr(), opts.BackendLabel())
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}

type quicPacketDumpHandler struct{}

func (quicPacketDumpHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	addressed, ok := msg.(udp.Addressed)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	packet, ok := addressed.Message.(quic.Packet)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	defer addressed.Release()
	payload := 0
	if packet.Payload != nil {
		payload = packet.Payload.ReadableBytes()
	}
	fmt.Printf("from=%s type=%d version=%s dcid=%s scid=%s pn=%d payload=%d\n",
		addressed.Addr, packet.Type, packet.Version, packet.DestinationID, packet.SourceID, packet.PacketNumber, payload)
}

func (quicPacketDumpHandler) ExceptionCaught(_ *channel.HandlerContext, err error) {
	fmt.Fprintln(os.Stderr, err)
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

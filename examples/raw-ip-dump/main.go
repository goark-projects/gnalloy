package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	ipcodec "goark.dev/gnalloy/codec/ip"
	"goark.dev/gnalloy/examples/internal/exampleconfig"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/raw"
)

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	opts := exampleconfig.Register(fs, "0.0.0.0")
	protocol := fs.Int("protocol", raw.ProtocolICMP, "raw socket protocol number")
	familyText := fs.String("family", "ipv4", "address family: ipv4 or ipv6")
	_ = fs.Parse(os.Args[1:])
	if err := opts.Resolve(); err != nil {
		fatal(err)
	}

	family := raw.FamilyIPv4
	if *familyText == "ipv6" {
		family = raw.FamilyIPv6
		if opts.Addr == "0.0.0.0" {
			opts.Addr = "::"
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	boss, workers, err := opts.NewGroups()
	if err != nil {
		fatal(err)
	}
	defer shutdown(boss)
	defer shutdown(workers)

	cfg := raw.DefaultConfig()
	cfg.Protocol = *protocol
	cfg.Family = family
	cfg.ReadBufferSize = opts.ReadBufferSize
	server, err := bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(raw.NewTransport(cfg)).
		ChildInitializer(func(ch channel.Channel) error {
			if err := ch.Pipeline().AddLast("ipDecoder", ipcodec.NewDecoder()); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("dump", rawIPDumpHandler{})
		}).
		BindContext(ctx, opts.Addr)
	if err != nil {
		fatal(fmt.Errorf("%w; raw socket needs Administrator or CAP_NET_RAW/root", err))
	}
	defer server.Close()

	fmt.Printf("gnalloy raw ip dump on %s protocol=%d backend=%s\n", server.Addr(), *protocol, opts.BackendLabel())
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}

type rawIPDumpHandler struct{}

func (rawIPDumpHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	addressed, ok := msg.(raw.Addressed)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	packet, ok := addressed.Message.(ipcodec.Packet)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	defer addressed.Release()
	payload := 0
	if packet.Payload != nil {
		payload = packet.Payload.ReadableBytes()
	}
	fmt.Printf("from=%s src=%s dst=%s version=%d proto=%d payload=%d\n",
		addressed.Addr, ipString(packet.Header.Source), ipString(packet.Header.Destination),
		packet.Header.Version, packet.Header.PayloadProtocol(), payload)
}

func (rawIPDumpHandler) ExceptionCaught(_ *channel.HandlerContext, err error) {
	fmt.Fprintln(os.Stderr, err)
}

func ipString(ip net.IP) string {
	if ip == nil {
		return ""
	}
	return ip.String()
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

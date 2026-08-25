package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	ipcodec "goark.dev/gnalloy/codec/ip"
	"goark.dev/gnalloy/examples/internal/exampleconfig"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/raw"
)

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	opts := exampleconfig.Register(fs, "0.0.0.0")
	targetText := fs.String("target", "127.0.0.1", "target ip address")
	sourceText := fs.String("source", "127.0.0.1", "source ip address in encoded ip header")
	protocol := fs.Int("protocol", 253, "custom ip protocol number")
	payloadText := fs.String("payload", "gnalloy-custom-ip", "custom protocol payload")
	_ = fs.Parse(os.Args[1:])
	if err := opts.Resolve(); err != nil {
		fatal(err)
	}
	target := net.ParseIP(*targetText)
	source := net.ParseIP(*sourceText)
	if target == nil || source == nil {
		fatal(fmt.Errorf("invalid source or target ip"))
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
	cfg.Protocol = raw.ProtocolRaw
	cfg.Family = raw.FamilyIPv4
	cfg.HeaderIncluded = true
	cfg.ReadBufferSize = opts.ReadBufferSize
	cfg.WriteBufferWatermark = opts.WriteBufferWatermark()

	done := make(chan error, 1)
	server, err := bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(raw.NewTransport(cfg)).
		ChildInitializer(func(ch channel.Channel) error {
			if err := ch.Pipeline().AddLast("rawPacketEncoder", raw.NewMessageToPacketEncoderFunc(func(any) bool { return false }, nil)); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("ipEncoder", ipcodec.NewEncoder()); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("sender", &customSender{
				target:   raw.Address{IP: target},
				source:   source,
				protocol: *protocol,
				payload:  []byte(*payloadText),
				done:     done,
			})
		}).
		BindContext(ctx, opts.Addr)
	if err != nil {
		fatal(fmt.Errorf("%w; raw socket needs Administrator or CAP_NET_RAW/root", err))
	}
	defer server.Close()

	if err := <-done; err != nil {
		fatal(err)
	}
	fmt.Printf("sent custom ip protocol=%d from=%s to=%s bytes=%d\n", *protocol, source, target, len(*payloadText))
}

type customSender struct {
	target   raw.Address
	source   net.IP
	protocol int
	payload  []byte
	done     chan<- error
}

func (s *customSender) ChannelActive(ctx *channel.HandlerContext) {
	payload := buffer.ByteBuf(buffer.NewHeapBuffer(0))
	if len(s.payload) > 0 {
		var err error
		payload, err = ctx.Channel().Allocator().Acquire(len(s.payload))
		if err != nil {
			s.done <- err
			return
		}
		if _, err := payload.WriteBytes(s.payload); err != nil {
			payload.Release()
			s.done <- err
			return
		}
	}
	packet := ipcodec.Packet{
		Header: ipcodec.Header{
			Version:     ipcodec.Version4,
			Protocol:    s.protocol,
			Source:      s.source,
			Destination: s.target.IP,
		},
		Payload: payload,
	}
	s.done <- ctx.Channel().WriteAndFlush(raw.Addressed{Message: packet, Addr: s.target, Protocol: raw.ProtocolRaw})
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

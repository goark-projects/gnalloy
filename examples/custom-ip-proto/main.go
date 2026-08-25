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
	"goark.dev/gnalloy/codec"
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
	formatText := fs.String("format", "ip", "wire format: ip or payload")
	_ = fs.Parse(os.Args[1:])
	if err := opts.Resolve(); err != nil {
		fatal(err)
	}
	target := net.ParseIP(*targetText)
	source := net.ParseIP(*sourceText)
	if target == nil || source == nil {
		fatal(fmt.Errorf("invalid source or target ip"))
	}
	format, rawProtocol, headerIncluded, err := resolveFormat(*formatText, *protocol)
	if err != nil {
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

	cfg := raw.DefaultConfig()
	cfg.Protocol = rawProtocol
	cfg.Family = raw.FamilyIPv4
	cfg.HeaderIncluded = headerIncluded
	cfg.ReadBufferSize = opts.ReadBufferSize
	cfg.WriteBufferWatermark = opts.WriteBufferWatermark()

	done := make(chan error, 1)
	server, err := bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(raw.NewTransport(cfg)).
		ChildInitializer(func(ch channel.Channel) error {
			protocolCodec := ipcodec.NewProtocolCodecFunc(
				ipcodec.ProtocolCodecConfig{
					Protocol:     *protocol,
					PacketFormat: format,
					Version:      ipcodec.Version4,
					Source:       source,
				},
				nil,
				nil,
				func(msg any) bool {
					_, ok := msg.(string)
					return ok
				},
				encodeStringPayload,
			)
			if err := ch.Pipeline().AddLast("customProtocol", protocolCodec); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("sender", &customSender{
				target:   raw.Address{IP: target},
				protocol: *protocol,
				payload:  *payloadText,
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
	fmt.Printf("sent custom ip protocol=%d format=%s from=%s to=%s bytes=%d\n", *protocol, *formatText, source, target, len(*payloadText))
}

type customSender struct {
	target   raw.Address
	protocol int
	payload  string
	done     chan<- error
}

func (s *customSender) ChannelActive(ctx *channel.HandlerContext) {
	s.done <- ctx.Channel().WriteAndFlush(raw.Addressed{Message: s.payload, Addr: s.target, Protocol: s.protocol})
}

func encodeStringPayload(ctx *channel.HandlerContext, msg any, out *codec.MessageList) error {
	text := msg.(string)
	if text == "" {
		out.Add(buffer.NewHeapBuffer(0))
		return nil
	}
	buf, err := ctx.Channel().Allocator().Acquire(len(text))
	if err != nil {
		return err
	}
	if _, err := buf.WriteBytes([]byte(text)); err != nil {
		buf.Release()
		return err
	}
	out.Add(buf)
	return nil
}

func resolveFormat(text string, protocol int) (ipcodec.PacketFormat, int, bool, error) {
	switch text {
	case "payload":
		return ipcodec.PacketFormatPayload, protocol, false, nil
	case "ip":
		return ipcodec.PacketFormatIP, raw.ProtocolRaw, true, nil
	default:
		return 0, 0, false, fmt.Errorf("invalid format %q", text)
	}
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

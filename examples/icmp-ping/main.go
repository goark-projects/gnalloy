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
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/icmp"
	"goark.dev/gnalloy/examples/internal/exampleconfig"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/raw"
)

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	opts := exampleconfig.Register(fs, "0.0.0.0")
	targetText := fs.String("target", "127.0.0.1", "target ip address")
	sourceText := fs.String("source-ip", "", "icmpv6 pseudo header source ip")
	payloadText := fs.String("payload", "gnalloy", "echo request payload")
	timeout := fs.Duration("timeout", 3*time.Second, "ping timeout")
	_ = fs.Parse(os.Args[1:])
	if err := opts.Resolve(); err != nil {
		fatal(err)
	}

	targetIP := net.ParseIP(*targetText)
	if targetIP == nil {
		fatal(fmt.Errorf("invalid target ip: %s", *targetText))
	}
	protocol := raw.ProtocolICMP
	family := raw.FamilyIPv4
	if targetIP.To4() == nil {
		protocol = raw.ProtocolICMPv6
		family = raw.FamilyIPv6
		if opts.Addr == "0.0.0.0" {
			opts.Addr = "::"
		}
	}
	sourceIP := net.ParseIP(*sourceText)
	if sourceIP == nil && family == raw.FamilyIPv6 {
		sourceIP = net.ParseIP(opts.Addr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	boss, workers, err := opts.NewGroups()
	if err != nil {
		fatal(err)
	}
	defer shutdown(boss)
	defer shutdown(workers)

	rawConfig := raw.DefaultConfig()
	rawConfig.Protocol = protocol
	rawConfig.Family = family
	rawConfig.ReadBufferSize = opts.ReadBufferSize
	rawConfig.WriteBufferWatermark = opts.WriteBufferWatermark()
	if opts.Mmap {
		rawConfig.AllocatorFactory = raw.NewMmapAllocatorFactory(buffer.MmapAllocatorConfig{
			BlockSize: opts.MmapBlockSize,
			Blocks:    opts.MmapBlocks,
		}, opts.MmapFallback)
	}

	result := make(chan error, 1)
	handler := &pingHandler{
		target:     raw.Address{IP: targetIP},
		protocol:   protocol,
		identifier: uint16(os.Getpid()),
		sequence:   uint16(time.Now().UnixNano()),
		payload:    []byte(*payloadText),
		start:      time.Now(),
		result:     result,
	}

	server, err := bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(raw.NewTransport(rawConfig)).
		ChildInitializer(func(ch channel.Channel) error {
			if err := ch.Pipeline().AddLast("rawPacketEncoder", raw.NewMessageToPacketEncoderFunc(func(any) bool { return false }, nil)); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("rawPacketDecoder", raw.NewPacketToMessageDecoderFunc(nil, decodeRawPayload)); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("icmpDecoder", icmp.NewDecoder()); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("ping", handler); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("icmpEncoder", icmp.NewEncoder(icmp.EncoderConfig{IPv6SourceIP: sourceIP}))
		}).
		BindContext(ctx, opts.Addr)
	if err != nil {
		fatal(fmt.Errorf("%w; raw socket needs Administrator or CAP_NET_RAW/root", err))
	}
	defer server.Close()

	fmt.Printf("gnalloy icmp ping %s from %s backend=%s\n", targetIP, server.Addr(), opts.BackendLabel())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	timer := time.NewTimer(*timeout)
	defer timer.Stop()

	select {
	case err := <-result:
		if err != nil {
			fatal(err)
		}
	case <-timer.C:
		fatal(fmt.Errorf("timeout waiting for icmp echo reply from %s", targetIP))
	case <-stop:
	}
}

func decodeRawPayload(_ *channel.HandlerContext, payload buffer.ByteBuf, out *codec.MessageList) error {
	frame, err := payload.Slice(payload.ReaderIndex(), payload.ReadableBytes())
	if err != nil {
		return err
	}
	out.Add(frame)
	return nil
}

type pingHandler struct {
	target     raw.Address
	protocol   int
	identifier uint16
	sequence   uint16
	payload    []byte
	start      time.Time
	result     chan<- error
}

func (h *pingHandler) ChannelActive(ctx *channel.HandlerContext) {
	payload := buffer.ByteBuf(buffer.NewHeapBuffer(0))
	if len(h.payload) > 0 {
		var err error
		payload, err = ctx.Channel().Allocator().Acquire(len(h.payload))
		if err != nil {
			h.done(err)
			return
		}
		if _, err := payload.WriteBytes(h.payload); err != nil {
			payload.Release()
			h.done(err)
			return
		}
	}
	msg := icmp.NewEchoRequest(h.protocol, h.identifier, h.sequence, payload)
	if err := ctx.Channel().WriteAndFlush(raw.Addressed{Message: msg, Addr: h.target, Protocol: h.protocol}); err != nil {
		h.done(err)
		return
	}
}

func (h *pingHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	addressed, ok := msg.(raw.Addressed)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	reply, ok := addressed.Message.(*icmp.Message)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	defer addressed.Release()
	if !reply.IsEchoReply() || reply.Identifier != h.identifier || reply.Sequence != h.sequence {
		return
	}
	fmt.Printf("icmp echo reply from %s bytes=%d time=%s\n", addressed.Addr, replyPayloadSize(reply), time.Since(h.start).Round(time.Microsecond))
	h.done(nil)
	_ = ctx.Pipeline().Close()
}

func (h *pingHandler) ExceptionCaught(_ *channel.HandlerContext, err error) {
	h.done(err)
}

func (h *pingHandler) done(err error) {
	select {
	case h.result <- err:
	default:
	}
}

func replyPayloadSize(msg *icmp.Message) int {
	if msg == nil || msg.Payload == nil {
		return 0
	}
	return msg.Payload.ReadableBytes()
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

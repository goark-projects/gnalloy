package udp_test

import (
	"context"
	"net"
	"testing"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/udp"
)

func TestBootstrapUDPResponsibilityChainEcho(t *testing.T) {
	boss, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         1,
		PollerConfig: transport.Config{Backend: transport.DefaultBackend()},
	})
	if err != nil {
		t.Fatal(err)
	}
	workers, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         1,
		PollerConfig: transport.Config{Backend: transport.DefaultBackend()},
	})
	if err != nil {
		_ = boss.Close()
		t.Fatal(err)
	}
	defer shutdownGroup(t, boss)
	defer shutdownGroup(t, workers)

	events := make(chan string, 4)
	server, err := bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(udp.NewTransport(udp.DefaultConfig())).
		ChildInitializer(func(ch channel.Channel) error {
			if err := ch.Pipeline().AddLast("datagramDecoder", udp.NewDatagramToMessageDecoder(udpPayloadDecoder{})); err != nil {
				return err
			}
			if err := ch.Pipeline().AddLast("echo", &udpEchoHandler{events: events}); err != nil {
				return err
			}
			return ch.Pipeline().AddLast("datagramEncoder", udp.NewMessageToDatagramEncoderFunc(func(any) bool { return false }, nil))
		}).
		Bind("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	if event := waitEvent(t, events); event != "active" {
		t.Fatalf("first event=%s, want active", event)
	}

	conn, err := net.Dial("udp", server.Addr())
	if err != nil {
		_ = server.Close()
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		_ = server.Close()
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		_ = server.Close()
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	if err != nil {
		_ = server.Close()
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "ping" {
		_ = server.Close()
		t.Fatalf("echo=%q, want ping", got)
	}
	if event := waitEvent(t, events); event != "read" {
		_ = server.Close()
		t.Fatalf("second event=%s, want read", event)
	}

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if event := waitEvent(t, events); event != "inactive" {
		t.Fatalf("third event=%s, want inactive", event)
	}
}

type udpPayloadDecoder struct{}

func (udpPayloadDecoder) AcceptDatagram(udp.Datagram) bool {
	return true
}

func (udpPayloadDecoder) DecodeDatagram(_ *channel.HandlerContext, payload buffer.ByteBuf, out *codec.MessageList) error {
	frame, err := payload.Slice(payload.ReaderIndex(), payload.ReadableBytes())
	if err != nil {
		return err
	}
	out.Add(frame)
	return nil
}

type udpEchoHandler struct {
	events chan<- string
}

func (h *udpEchoHandler) ChannelActive(ctx *channel.HandlerContext) {
	h.events <- "active"
	ctx.FireChannelActive()
}

func (h *udpEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
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
	h.events <- "read"
	if err := ctx.Channel().WriteAndFlush(udp.Addressed{Message: payload, Addr: addressed.Addr}); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (h *udpEchoHandler) ChannelInactive(ctx *channel.HandlerContext) {
	h.events <- "inactive"
	ctx.FireChannelInactive()
}

func (h *udpEchoHandler) ExceptionCaught(_ *channel.HandlerContext, err error) {
	select {
	case h.events <- "error:" + err.Error():
	default:
	}
}

func waitEvent(t *testing.T, events <-chan string) string {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for udp pipeline event")
		return ""
	}
}

func shutdownGroup(t *testing.T, group *transport.EventLoopGroup) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := group.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown event loop group: %v", err)
	}
}

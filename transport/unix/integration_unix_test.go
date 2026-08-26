//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package unix

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

func TestUnixTransportEchoSmoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	boss, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{Size: 1, PollerConfig: transport.Config{Backend: transport.BackendStd}})
	if err != nil {
		t.Fatal(err)
	}
	defer boss.Shutdown(context.Background())
	workers, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{Size: 1, PollerConfig: transport.Config{Backend: transport.BackendStd}})
	if err != nil {
		t.Fatal(err)
	}
	defer workers.Shutdown(context.Background())

	path := filepath.Join(t.TempDir(), "gnalloy.sock")
	server, err := bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(NewTransport(DefaultConfig())).
		ChildInitializer(func(ch channel.Channel) error {
			return ch.Pipeline().AddLast("echo", echoHandler{})
		}).
		BindContext(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	clientGroup, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{Size: 1, PollerConfig: transport.Config{Backend: transport.BackendStd}})
	if err != nil {
		t.Fatal(err)
	}
	defer clientGroup.Shutdown(context.Background())

	received := make(chan string, 1)
	client, err := bootstrap.NewDialer().
		Group(clientGroup).
		Transport(NewTransport(DefaultConfig())).
		Initializer(func(ch channel.Channel) error {
			return ch.Pipeline().AddLast("capture", captureHandler{out: received})
		}).
		DialContext(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	msg := buffer.NewHeapBuffer(8)
	if _, err := msg.WriteBytes([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	if err := client.WriteAndFlush(msg); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-received:
		if got != "ping" {
			t.Fatalf("echo=%q, want ping", got)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

type echoHandler struct{}

func (echoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	_ = ctx.Write(msg)
	_ = ctx.Flush()
}

type captureHandler struct {
	out chan<- string
}

func (h captureHandler) ChannelRead(_ *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return
	}
	h.out <- string(buf.Bytes())
	buf.Release()
}

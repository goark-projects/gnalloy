package local

import (
	"context"
	"errors"
	"testing"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	transportcore "goark.dev/gnalloy/transport"
)

func TestTransportImplementsBootstrapContracts(t *testing.T) {
	var _ bootstrap.ServerTransport = (*Transport)(nil)
	var _ bootstrap.ClientTransport = (*Transport)(nil)
}

func TestDialerPairsClientAndServerPipelines(t *testing.T) {
	group := newLocalTestGroup(t)
	serverReady := make(chan channel.Channel, 1)
	clientReads := newByteRecorder()

	server, err := bootstrap.NewServerBootstrap().
		Group(group, group).
		Transport(NewTransport(Config{})).
		ChildInitializer(func(ch channel.Channel) error {
			serverReady <- ch
			return ch.Pipeline().AddLast("echo", localEchoHandler{})
		}).
		Bind("local:test-echo")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	client, err := bootstrap.NewDialer().
		Group(group).
		Transport(NewTransport(Config{})).
		Initializer(func(ch channel.Channel) error {
			return ch.Pipeline().AddLast("recorder", clientReads)
		}).
		Dial("local:test-echo")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	select {
	case <-serverReady:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for accepted local channel")
	}

	msg := localBuffer(t, client.Allocator(), "ping")
	if err := client.WriteAndFlush(msg); err != nil {
		t.Fatal(err)
	}
	clientReads.wait(t, "ping")
}

func TestBindRejectsDuplicateAddress(t *testing.T) {
	group := newLocalTestGroup(t)
	first, err := bootstrap.NewServerBootstrap().
		Group(group, group).
		Transport(NewTransport(Config{})).
		ChildInitializer(func(channel.Channel) error { return nil }).
		Bind("local:duplicate")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	_, err = bootstrap.NewServerBootstrap().
		Group(group, group).
		Transport(NewTransport(Config{})).
		ChildInitializer(func(channel.Channel) error { return nil }).
		Bind("local:duplicate")
	if !errors.Is(err, ErrAddressInUse) {
		t.Fatalf("err=%v, want %v", err, ErrAddressInUse)
	}
}

func TestDialFailsWhenAddressIsUnbound(t *testing.T) {
	group := newLocalTestGroup(t)
	_, err := bootstrap.NewDialer().
		Group(group).
		Transport(NewTransport(Config{})).
		Dial("local:missing")
	if !errors.Is(err, ErrServerNotFound) {
		t.Fatalf("err=%v, want %v", err, ErrServerNotFound)
	}
}

type localEchoHandler struct{}

func (localEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	_ = ctx.WriteAndFlush(msg)
}

type byteRecorder struct {
	reads chan string
}

func newByteRecorder() *byteRecorder {
	return &byteRecorder{reads: make(chan string, 4)}
}

func (r *byteRecorder) ChannelRead(_ *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return
	}
	defer buf.Release()
	r.reads <- string(buf.Bytes())
}

func (r *byteRecorder) wait(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-r.reads:
		if got != want {
			t.Fatalf("read=%q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for %q", want)
	}
}

func localBuffer(t *testing.T, alloc buffer.Allocator, value string) buffer.ByteBuf {
	t.Helper()
	buf, err := alloc.Acquire(len(value))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buf.WriteBytes([]byte(value)); err != nil {
		buf.Release()
		t.Fatal(err)
	}
	return buf
}

func newLocalTestGroup(t *testing.T) *transportcore.EventLoopGroup {
	t.Helper()
	group, err := transportcore.NewEventLoopGroup(transportcore.EventLoopGroupConfig{
		Size:         1,
		PollerConfig: transportcore.Config{Backend: transportcore.BackendMemory},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = group.Shutdown(ctx)
	})
	return group
}

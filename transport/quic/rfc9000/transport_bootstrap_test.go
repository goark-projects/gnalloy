package rfc9000

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	transportcore "goark.dev/gnalloy/transport"
)

func TestTransportBootstrapDialerEchoOverQUICStream(t *testing.T) {
	const alpn = "gnalloy-bootstrap-test"
	cert, roots := testCertificate(t, "gnalloy.local")
	boss := newBootstrapTestGroup(t)
	workers := newBootstrapTestGroup(t)
	clientGroup := newBootstrapTestGroup(t)

	server, err := bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(NewTransport(Config{
			TLS:        &tls.Config{Certificates: []tls.Certificate{cert}},
			NextProtos: []string{alpn},
		})).
		ChildInitializer(func(ch channel.Channel) error {
			return ch.Pipeline().AddLast("echo", streamEchoHandler{})
		}).
		Bind("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	recorder := newStreamBootstrapRecorder()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := bootstrap.NewDialer().
		Group(clientGroup).
		Transport(NewTransport(Config{
			TLS: &tls.Config{
				RootCAs:    roots,
				ServerName: "gnalloy.local",
			},
			NextProtos: []string{alpn},
		})).
		Initializer(func(ch channel.Channel) error {
			return ch.Pipeline().AddLast("recorder", recorder)
		}).
		DialContext(ctx, server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ch.Close() })

	out, err := ch.Allocator().Acquire(4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := out.WriteBytes([]byte("ping")); err != nil {
		out.Release()
		t.Fatal(err)
	}
	if err := ch.WriteAndFlush(out); err != nil {
		t.Fatal(err)
	}
	recorder.waitPayload(t, "ping")
}

type streamEchoHandler struct{}

func (streamEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := ctx.Channel().WriteAndFlush(buf); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

type streamBootstrapRecorder struct {
	payloads chan string
}

func newStreamBootstrapRecorder() *streamBootstrapRecorder {
	return &streamBootstrapRecorder{payloads: make(chan string, 1)}
}

func (r *streamBootstrapRecorder) ChannelRead(_ *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return
	}
	defer buf.Release()
	select {
	case r.payloads <- string(buf.Bytes()):
	default:
	}
}

func (r *streamBootstrapRecorder) waitPayload(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-r.payloads:
		if got != want {
			t.Fatalf("payload=%q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for payload %q", want)
	}
}

func newBootstrapTestGroup(t *testing.T) *transportcore.EventLoopGroup {
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

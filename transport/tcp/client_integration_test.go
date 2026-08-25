package tcp_test

import (
	"context"
	"testing"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/tcp"
)

func TestTCPDialerEcho(t *testing.T) {
	skipUnsupportedTCP(t)
	server := startTCPServer(t, func(ch channel.Channel) error {
		return ch.Pipeline().AddLast("echo", &recordingEchoHandler{recorder: newLifecycleRecorder()})
	})

	group, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         1,
		PollerConfig: transport.Config{Backend: transport.DefaultBackend()},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shutdownGroup(t, group) })

	recorder := newClientReadRecorder()
	ch, err := bootstrap.NewDialer().
		Group(group).
		Transport(tcp.NewTransport(tcp.DefaultConfig())).
		Initializer(func(ch channel.Channel) error {
			return ch.Pipeline().AddLast("recorder", recorder)
		}).
		DialContext(context.Background(), server.Addr())
	if err != nil {
		t.Fatal(err)
	}

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
	if err := ch.CloseFuture().Await(); err != nil {
		t.Fatal(err)
	}
}

type clientReadRecorder struct {
	*lifecycleRecorder
	payloads []string
}

func newClientReadRecorder() *clientReadRecorder {
	return &clientReadRecorder{lifecycleRecorder: newLifecycleRecorder()}
}

func (r *clientReadRecorder) ChannelActive(ctx *channel.HandlerContext) {
	r.record("active")
	ctx.FireChannelActive()
}

func (r *clientReadRecorder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	r.mu.Lock()
	r.payloads = append(r.payloads, string(buf.Bytes()))
	r.mu.Unlock()
	buf.Release()
}

func (r *clientReadRecorder) waitPayload(t *testing.T, want string) {
	t.Helper()
	waitFor(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, payload := range r.payloads {
			if payload == want {
				return true
			}
		}
		return false
	}, "client payload")
}

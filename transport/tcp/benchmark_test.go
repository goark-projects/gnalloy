package tcp_test

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"runtime"
	"testing"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/tcp"
)

func BenchmarkNativeTCPEchoRoundTrip(b *testing.B) {
	skipUnsupportedTCPBenchmark(b)
	server := startTCPBenchmarkServer(b, tcp.DefaultConfig(), func(ch channel.Channel) error {
		return ch.Pipeline().AddLast("echo", benchmarkEchoHandler{})
	})
	conn := dialTCPBenchmark(b, server.Addr())
	defer conn.Close()

	payload := []byte("ping")
	got := make([]byte, len(payload))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := conn.Write(payload); err != nil {
			b.Fatal(err)
		}
		if _, err := io.ReadFull(conn, got); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLengthFieldTCPRoundTrip(b *testing.B) {
	skipUnsupportedTCPBenchmark(b)
	server := startTCPBenchmarkServer(b, tcp.DefaultConfig(), func(ch channel.Channel) error {
		decoder, err := codec.NewLengthFieldBasedFrameDecoder(1<<20, 0, 4, 0, 4, buffer.BigEndian)
		if err != nil {
			return err
		}
		if err := ch.Pipeline().AddLast("frame", decoder); err != nil {
			return err
		}
		return ch.Pipeline().AddLast("echo", benchmarkLengthFieldEchoHandler{})
	})
	conn := dialTCPBenchmark(b, server.Addr())
	defer conn.Close()

	frame := encodeFrame([]byte("ping"))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := conn.Write(frame); err != nil {
			b.Fatal(err)
		}
		if got := readBenchmarkFrame(b, conn); string(got) != "ping" {
			b.Fatalf("echo=%q, want ping", got)
		}
	}
}

func startTCPBenchmarkServer(b *testing.B, cfg tcp.Config, initializer bootstrap.ChildInitializer) bootstrap.Server {
	b.Helper()
	boss, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         1,
		PollerConfig: transport.Config{Backend: transport.DefaultBackend()},
	})
	if err != nil {
		b.Fatal(err)
	}
	workers, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         runtime.GOMAXPROCS(0),
		PollerConfig: transport.Config{Backend: transport.DefaultBackend()},
	})
	if err != nil {
		_ = boss.Close()
		b.Fatal(err)
	}

	server, err := bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(tcp.NewTransport(cfg)).
		ChildInitializer(initializer).
		BindContext(context.Background(), "127.0.0.1:0")
	if err != nil {
		_ = workers.Close()
		_ = boss.Close()
		b.Fatal(err)
	}

	b.Cleanup(func() {
		_ = server.Close()
		shutdownBenchmarkGroup(b, workers)
		shutdownBenchmarkGroup(b, boss)
	})
	return server
}

func dialTCPBenchmark(b *testing.B, address string) net.Conn {
	b.Helper()
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		b.Fatal(err)
	}
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		_ = conn.Close()
		b.Fatal(err)
	}
	return conn
}

func shutdownBenchmarkGroup(b *testing.B, group *transport.EventLoopGroup) {
	b.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := group.Shutdown(ctx); err != nil {
		b.Fatal(err)
	}
}

type benchmarkEchoHandler struct{}

func (benchmarkEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := ctx.Channel().WriteAndFlush(buf); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (benchmarkEchoHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
	_ = ctx.Pipeline().Close()
}

type benchmarkLengthFieldEchoHandler struct{}

func (benchmarkLengthFieldEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	frame, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	defer frame.Release()

	payload := frame.Bytes()
	out, err := ctx.Channel().Allocator().Acquire(4 + len(payload))
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	binary.BigEndian.PutUint32(out.WritableBytesView()[:4], uint32(len(payload)))
	if err := out.AdvanceWriter(4); err != nil {
		out.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if _, err := out.WriteBytes(payload); err != nil {
		out.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Channel().WriteAndFlush(out); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (benchmarkLengthFieldEchoHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
	_ = ctx.Pipeline().Close()
}

func readBenchmarkFrame(b *testing.B, r io.Reader) []byte {
	b.Helper()
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		b.Fatal(err)
	}
	size := binary.BigEndian.Uint32(header[:])
	out := make([]byte, size)
	if _, err := io.ReadFull(r, out); err != nil {
		b.Fatal(err)
	}
	return out
}

func skipUnsupportedTCPBenchmark(b *testing.B) {
	b.Helper()
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "netbsd", "openbsd", "dragonfly", "windows":
	default:
		b.Skipf("native tcp is unsupported on %s", runtime.GOOS)
	}
}

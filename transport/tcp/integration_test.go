package tcp_test

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/tcp"
)

func TestNativeTCPEchoLifecycle(t *testing.T) {
	skipUnsupportedTCP(t)
	recorder := newLifecycleRecorder()
	server := startTCPServer(t, func(ch channel.Channel) error {
		return ch.Pipeline().AddLast("echo", &recordingEchoHandler{recorder: recorder})
	})

	conn := dialTCP(t, server.Addr())
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	var got [4]byte
	if _, err := io.ReadFull(conn, got[:]); err != nil {
		t.Fatal(err)
	}
	if string(got[:]) != "ping" {
		t.Fatalf("echo=%q, want ping", got[:])
	}
	_ = conn.Close()

	recorder.wait(t, "active")
	recorder.wait(t, "read")
	recorder.wait(t, "inactive")
	if recorder.exceptionCount() != 0 {
		t.Fatalf("exceptions=%d", recorder.exceptionCount())
	}
	if gotEvents := recorder.prefix(3); !reflect.DeepEqual(gotEvents, []string{"active", "read", "inactive"}) {
		t.Fatalf("events=%v", recorder.snapshot())
	}
}

func TestLengthFieldTCPHandlesSplitAndStickyFrames(t *testing.T) {
	skipUnsupportedTCP(t)
	server := startTCPServer(t, func(ch channel.Channel) error {
		decoder, err := codec.NewLengthFieldBasedFrameDecoder(1<<20, 0, 4, 0, 4, buffer.BigEndian)
		if err != nil {
			return err
		}
		if err := ch.Pipeline().AddLast("frame", decoder); err != nil {
			return err
		}
		return ch.Pipeline().AddLast("echo", lengthFieldEchoHandler{})
	})

	conn := dialTCP(t, server.Addr())
	defer conn.Close()

	frame := encodeFrame([]byte("split"))
	if _, err := conn.Write(frame[:2]); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(frame[2:]); err != nil {
		t.Fatal(err)
	}
	if got := readFrame(t, conn); string(got) != "split" {
		t.Fatalf("split echo=%q", got)
	}

	sticky := append(encodeFrame([]byte("one")), encodeFrame([]byte("two"))...)
	if _, err := conn.Write(sticky); err != nil {
		t.Fatal(err)
	}
	if got := readFrame(t, conn); string(got) != "one" {
		t.Fatalf("first sticky echo=%q", got)
	}
	if got := readFrame(t, conn); string(got) != "two" {
		t.Fatalf("second sticky echo=%q", got)
	}
}

func TestNativeTCPRepeatedConnectClose(t *testing.T) {
	skipUnsupportedTCP(t)
	recorder := newLifecycleRecorder()
	server := startTCPServer(t, func(ch channel.Channel) error {
		return ch.Pipeline().AddLast("echo", &recordingEchoHandler{recorder: recorder})
	})

	const clients = 16
	for i := 0; i < clients; i++ {
		conn := dialTCP(t, server.Addr())
		if _, err := conn.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		var got [1]byte
		if _, err := io.ReadFull(conn, got[:]); err != nil {
			t.Fatal(err)
		}
		_ = conn.Close()
	}

	recorder.waitCount(t, "inactive", clients)
	if recorder.count("active") != clients || recorder.count("read") != clients {
		t.Fatalf("events=%v", recorder.snapshot())
	}
}

func TestNativeTCPLongConnectionsStability(t *testing.T) {
	skipUnsupportedTCP(t)
	recorder := newLifecycleRecorder()
	server := startTCPServerWithConfig(t, 1, 4, tcp.DefaultConfig(), func(ch channel.Channel) error {
		return ch.Pipeline().AddLast("echo", &recordingEchoHandler{recorder: recorder})
	})
	activeServer, ok := server.(interface{ ActiveConnectionCount() int })
	if !ok {
		t.Fatal("server does not expose active connection count")
	}

	const (
		clients  = 32
		messages = 8
	)
	conns := make([]net.Conn, clients)
	for i := 0; i < clients; i++ {
		conns[i] = dialTCP(t, server.Addr())
	}
	defer func() {
		for _, conn := range conns {
			if conn != nil {
				_ = conn.Close()
			}
		}
	}()
	waitFor(t, func() bool { return activeServer.ActiveConnectionCount() == clients }, "all long connections to become active")

	var wg sync.WaitGroup
	errCh := make(chan error, clients)
	for i := 0; i < clients; i++ {
		conn := conns[i]
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			payload := []byte{byte(clientID), 'p', 'i', 'n', 'g'}
			reply := make([]byte, len(payload))
			for j := 0; j < messages; j++ {
				payload[0] = byte(clientID + j)
				if _, err := conn.Write(payload); err != nil {
					errCh <- err
					return
				}
				if _, err := io.ReadFull(conn, reply); err != nil {
					errCh <- err
					return
				}
				if string(reply) != string(payload) {
					errCh <- io.ErrUnexpectedEOF
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, conn := range conns {
		_ = conn.Close()
	}
	recorder.waitCount(t, "inactive", clients)
	waitFor(t, func() bool { return activeServer.ActiveConnectionCount() == 0 }, "all long connections to become inactive")
	if recorder.count("read") != clients*messages {
		t.Fatalf("read events=%d, want %d", recorder.count("read"), clients*messages)
	}
	if recorder.exceptionCount() != 0 {
		t.Fatalf("exceptions=%d", recorder.exceptionCount())
	}
}

func TestTCPServerCloseDrainsIdleLongConnections(t *testing.T) {
	skipUnsupportedTCP(t)
	recorder := newLifecycleRecorder()
	server := startTCPServerWithConfig(t, 1, 4, tcp.DefaultConfig(), func(ch channel.Channel) error {
		return ch.Pipeline().AddLast("echo", &recordingEchoHandler{recorder: recorder})
	})
	activeServer, ok := server.(interface{ ActiveConnectionCount() int })
	if !ok {
		t.Fatal("server does not expose active connection count")
	}

	const clients = 24
	conns := make([]net.Conn, clients)
	for i := 0; i < clients; i++ {
		conns[i] = dialTCP(t, server.Addr())
	}
	defer func() {
		for _, conn := range conns {
			if conn != nil {
				_ = conn.Close()
			}
		}
	}()
	waitFor(t, func() bool { return activeServer.ActiveConnectionCount() == clients }, "idle connections to become active")

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	recorder.waitCount(t, "inactive", clients)
	waitFor(t, func() bool { return activeServer.ActiveConnectionCount() == 0 }, "idle connections to drain after server close")
}

func TestTCPServerTracksAndClosesActiveChildren(t *testing.T) {
	skipUnsupportedTCP(t)
	recorder := newLifecycleRecorder()
	server := startTCPServer(t, func(ch channel.Channel) error {
		return ch.Pipeline().AddLast("echo", &recordingEchoHandler{recorder: recorder})
	})
	activeServer, ok := server.(interface{ ActiveConnectionCount() int })
	if !ok {
		t.Fatal("server does not expose active connection count")
	}

	conn := dialTCP(t, server.Addr())
	if _, err := conn.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	var got [1]byte
	if _, err := io.ReadFull(conn, got[:]); err != nil {
		t.Fatal(err)
	}
	recorder.wait(t, "active")
	waitFor(t, func() bool { return activeServer.ActiveConnectionCount() == 1 }, "active connection count to become 1")

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return activeServer.ActiveConnectionCount() == 0 }, "active connection count to become 0")
	recorder.wait(t, "inactive")
	_ = conn.Close()
}

func TestTCPAllocatorFactoryIsBoundPerWorker(t *testing.T) {
	skipUnsupportedTCP(t)

	var calls atomic.Int32
	var mu sync.Mutex
	seen := make(map[transport.EventLoopID]bool)

	cfg := tcp.DefaultConfig()
	cfg.AllocatorFactory = func(loop *transport.EventLoop) (buffer.Allocator, error) {
		calls.Add(1)
		mu.Lock()
		seen[loop.ID()] = true
		mu.Unlock()
		return buffer.NewHeapAllocator(), nil
	}

	recorder := newLifecycleRecorder()
	server := startTCPServerWithConfig(t, 1, 2, cfg, func(ch channel.Channel) error {
		return ch.Pipeline().AddLast("echo", &recordingEchoHandler{recorder: recorder})
	})

	for i := 0; i < 4; i++ {
		conn := dialTCP(t, server.Addr())
		if _, err := conn.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		var got [1]byte
		if _, err := io.ReadFull(conn, got[:]); err != nil {
			t.Fatal(err)
		}
		_ = conn.Close()
	}

	recorder.waitCount(t, "read", 4)
	if calls.Load() != 2 {
		t.Fatalf("allocator factory calls=%d, want 2", calls.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if !seen[0] || !seen[1] {
		t.Fatalf("allocator loops=%v, want worker loops 0 and 1", seen)
	}
}

func TestTCPServerExposesAllocatorStats(t *testing.T) {
	skipUnsupportedTCP(t)

	recorder := newLifecycleRecorder()
	server := startTCPServerWithConfig(t, 1, 1, tcp.DefaultConfig(), func(ch channel.Channel) error {
		return ch.Pipeline().AddLast("echo", &recordingEchoHandler{recorder: recorder})
	})
	statsServer, ok := server.(interface {
		AllocatorStats() []buffer.AllocatorStats
	})
	if !ok {
		t.Fatal("server does not expose allocator stats")
	}

	conn := dialTCP(t, server.Addr())
	if _, err := conn.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	var got [1]byte
	if _, err := io.ReadFull(conn, got[:]); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	recorder.wait(t, "read")

	stats := statsServer.AllocatorStats()
	if len(stats) != 1 {
		t.Fatalf("stats len=%d, want 1", len(stats))
	}
	if stats[0].OffHeap {
		t.Fatalf("heap allocator stats marked off-heap: %+v", stats[0])
	}
}

func TestTCPServerCloseClosesCachedAllocatorsOnce(t *testing.T) {
	skipUnsupportedTCP(t)

	var closes atomic.Int32
	cfg := tcp.DefaultConfig()
	cfg.AllocatorFactory = func(*transport.EventLoop) (buffer.Allocator, error) {
		return &countingAllocator{base: buffer.NewHeapAllocator(), closes: &closes}, nil
	}

	recorder := newLifecycleRecorder()
	server := startTCPServerWithConfig(t, 1, 2, cfg, func(ch channel.Channel) error {
		return ch.Pipeline().AddLast("echo", &recordingEchoHandler{recorder: recorder})
	})

	for i := 0; i < 2; i++ {
		conn := dialTCP(t, server.Addr())
		if _, err := conn.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		var got [1]byte
		if _, err := io.ReadFull(conn, got[:]); err != nil {
			t.Fatal(err)
		}
		_ = conn.Close()
	}

	recorder.waitCount(t, "inactive", 2)
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if got := closes.Load(); got != 2 {
		t.Fatalf("allocator closes=%d, want 2", got)
	}
}

func TestTCPServerCloseReleasesAllocatorsAfterActiveChildren(t *testing.T) {
	skipUnsupportedTCP(t)

	var (
		closeObservedActive atomic.Int32
		closes              atomic.Int32
		serverRef           atomic.Value
	)
	cfg := tcp.DefaultConfig()
	cfg.AllocatorFactory = func(*transport.EventLoop) (buffer.Allocator, error) {
		return &observingAllocator{
			base:   buffer.NewHeapAllocator(),
			closes: &closes,
			onClose: func() {
				if srv, ok := serverRef.Load().(interface{ ActiveConnectionCount() int }); ok && srv.ActiveConnectionCount() != 0 {
					closeObservedActive.Add(1)
				}
			},
		}, nil
	}

	recorder := newLifecycleRecorder()
	server := startTCPServerWithConfig(t, 1, 1, cfg, func(ch channel.Channel) error {
		return ch.Pipeline().AddLast("echo", &recordingEchoHandler{recorder: recorder})
	})
	serverRef.Store(server)

	conn := dialTCP(t, server.Addr())
	if _, err := conn.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	var got [1]byte
	if _, err := io.ReadFull(conn, got[:]); err != nil {
		t.Fatal(err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()

	if closes.Load() != 1 {
		t.Fatalf("allocator closes=%d, want 1", closes.Load())
	}
	if closeObservedActive.Load() != 0 {
		t.Fatal("allocator closed while active children were still registered")
	}
}

func TestReusePortBindsOneListenerPerBoss(t *testing.T) {
	skipUnsupportedTCP(t)
	skipUnsupportedReusePort(t)

	cfg := tcp.DefaultConfig()
	cfg.ReusePort = true
	server := startTCPServerWithConfig(t, 2, 1, cfg, func(ch channel.Channel) error {
		return ch.Pipeline().AddLast("discard", discardHandler{})
	})

	listeners, ok := server.(interface{ ListenerCount() int })
	if !ok {
		t.Fatal("server does not expose listener count")
	}
	if got := listeners.ListenerCount(); got != 2 {
		t.Fatalf("listeners=%d, want 2", got)
	}
}

func TestMmapAllocatorFactoryFallback(t *testing.T) {
	factory := tcp.NewMmapAllocatorFactory(buffer.MmapAllocatorConfig{
		BlockSize: 1024,
		Blocks:    2,
	}, true)
	alloc, err := factory(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer alloc.Close()

	buf, err := alloc.Acquire(16)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buf.WriteBytes([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if string(buf.Bytes()) != "ok" {
		t.Fatalf("buf=%q, want ok", buf.Bytes())
	}
	buf.Release()
}

func TestTCPServerRejectsFixedBuffersWithoutIOUringMmap(t *testing.T) {
	skipUnsupportedTCP(t)
	boss, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         1,
		PollerConfig: transport.Config{Backend: transport.DefaultBackend()},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownGroup(t, boss)
	workers, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         1,
		PollerConfig: transport.Config{Backend: transport.DefaultBackend()},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownGroup(t, workers)

	cfg := tcp.DefaultConfig()
	cfg.IOUringFixedBuffers = true
	_, err = bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(tcp.NewTransport(cfg)).
		ChildInitializer(func(ch channel.Channel) error {
			return ch.Pipeline().AddLast("discard", discardHandler{})
		}).
		BindContext(context.Background(), "127.0.0.1:0")
	if !errors.Is(err, tcp.ErrUnsupportedFixedBuffers) {
		t.Fatalf("bind err=%v, want %v", err, tcp.ErrUnsupportedFixedBuffers)
	}
}

func startTCPServer(t *testing.T, initializer bootstrap.ChildInitializer) bootstrap.Server {
	t.Helper()
	return startTCPServerWithConfig(t, 1, 1, tcp.DefaultConfig(), initializer)
}

func startTCPServerWithConfig(t *testing.T, bossSize int, workerSize int, cfg tcp.Config, initializer bootstrap.ChildInitializer) bootstrap.Server {
	t.Helper()
	boss, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         bossSize,
		PollerConfig: transport.Config{Backend: transport.DefaultBackend()},
	})
	if err != nil {
		t.Fatal(err)
	}
	workers, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         workerSize,
		PollerConfig: transport.Config{Backend: transport.DefaultBackend()},
	})
	if err != nil {
		_ = boss.Close()
		t.Fatal(err)
	}

	server, err := bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(tcp.NewTransport(cfg)).
		ChildInitializer(initializer).
		BindContext(context.Background(), "127.0.0.1:0")
	if err != nil {
		_ = workers.Close()
		_ = boss.Close()
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = server.Close()
		shutdownGroup(t, workers)
		shutdownGroup(t, boss)
	})
	return server
}

func dialTCP(t *testing.T, address string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	return conn
}

func shutdownGroup(t *testing.T, group *transport.EventLoopGroup) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := group.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

type lifecycleRecorder struct {
	mu     sync.Mutex
	events []string
}

func newLifecycleRecorder() *lifecycleRecorder {
	return &lifecycleRecorder{}
}

func (r *lifecycleRecorder) record(name string) {
	r.mu.Lock()
	r.events = append(r.events, name)
	r.mu.Unlock()
}

func (r *lifecycleRecorder) wait(t *testing.T, name string) {
	t.Helper()
	r.waitCount(t, name, 1)
}

func (r *lifecycleRecorder) waitCount(t *testing.T, name string, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if r.count(name) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s count=%d events=%v", name, want, r.snapshot())
}

func (r *lifecycleRecorder) count(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, ev := range r.events {
		if ev == name {
			count++
		}
	}
	return count
}

func (r *lifecycleRecorder) exceptionCount() int {
	return r.count("exception")
}

func (r *lifecycleRecorder) prefix(n int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n > len(r.events) {
		n = len(r.events)
	}
	out := make([]string, n)
	copy(out, r.events[:n])
	return out
}

func (r *lifecycleRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.events))
	copy(out, r.events)
	return out
}

func waitFor(t *testing.T, ok func() bool, name string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", name)
}

type recordingEchoHandler struct {
	recorder *lifecycleRecorder
}

func (h *recordingEchoHandler) ChannelActive(ctx *channel.HandlerContext) {
	h.recorder.record("active")
	ctx.FireChannelActive()
}

func (h *recordingEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	h.recorder.record("read")
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := ctx.Channel().WriteAndFlush(buf); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (h *recordingEchoHandler) ChannelInactive(ctx *channel.HandlerContext) {
	h.recorder.record("inactive")
	ctx.FireChannelInactive()
}

func (h *recordingEchoHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
	h.recorder.record("exception")
	_ = ctx.Pipeline().Close()
}

type lengthFieldEchoHandler struct{}

func (lengthFieldEchoHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
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

func (lengthFieldEchoHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
	_ = ctx.Pipeline().Close()
}

func encodeFrame(payload []byte) []byte {
	out := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(out[:4], uint32(len(payload)))
	copy(out[4:], payload)
	return out
}

func readFrame(t *testing.T, r io.Reader) []byte {
	t.Helper()
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		t.Fatal(err)
	}
	size := binary.BigEndian.Uint32(header[:])
	out := make([]byte, size)
	if _, err := io.ReadFull(r, out); err != nil {
		t.Fatal(err)
	}
	return out
}

func skipUnsupportedTCP(t *testing.T) {
	t.Helper()
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "netbsd", "openbsd", "dragonfly", "windows":
	default:
		t.Skipf("native tcp is unsupported on %s", runtime.GOOS)
	}
}

func skipUnsupportedReusePort(t *testing.T) {
	t.Helper()
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "netbsd", "openbsd", "dragonfly":
	default:
		t.Skipf("SO_REUSEPORT is unsupported on %s", runtime.GOOS)
	}
}

type discardHandler struct{}

func (discardHandler) ChannelRead(_ *channel.HandlerContext, msg any) {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		buf.Release()
	}
}

type countingAllocator struct {
	base   buffer.Allocator
	closes *atomic.Int32
}

func (a *countingAllocator) Acquire(size int) (buffer.ByteBuf, error) {
	return a.base.Acquire(size)
}

func (a *countingAllocator) Release(buf *buffer.DirectByteBuf) {
	a.base.Release(buf)
}

func (a *countingAllocator) Close() error {
	a.closes.Add(1)
	return a.base.Close()
}

type observingAllocator struct {
	base    buffer.Allocator
	closes  *atomic.Int32
	onClose func()
}

func (a *observingAllocator) Acquire(size int) (buffer.ByteBuf, error) {
	return a.base.Acquire(size)
}

func (a *observingAllocator) Release(buf *buffer.DirectByteBuf) {
	a.base.Release(buf)
}

func (a *observingAllocator) Close() error {
	a.closes.Add(1)
	if a.onClose != nil {
		a.onClose()
	}
	return a.base.Close()
}

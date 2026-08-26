package l2

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

func TestDialerWritesByteBufAndReadsL2Frame(t *testing.T) {
	driver := newFakeDriver()
	group := newL2TestGroup(t)
	recorder := newFrameRecorder()

	ch, err := bootstrap.NewDialer().
		Group(group).
		Transport(NewTransport(Config{Driver: driver, InterfaceName: "eth-test"})).
		Initializer(func(ch channel.Channel) error {
			return ch.Pipeline().AddLast("recorder", recorder)
		}).
		Dial("eth-test")
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()

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
	if got := string(driver.endpoint.waitWrite(t)); got != "ping" {
		t.Fatalf("write=%q, want ping", got)
	}

	driver.endpoint.reads <- []byte("pong")
	recorder.waitPayload(t, "pong")
}

func TestDialerPassesNormalizedConfigToDriver(t *testing.T) {
	driver := newFakeDriver()
	group := newL2TestGroup(t)

	ch, err := bootstrap.NewDialer().
		Group(group).
		Transport(NewTransport(Config{
			Driver:      driver,
			EtherType:   0x88b5,
			Promiscuous: true,
		})).
		Dial("eth-auto")
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()

	got := driver.lastConfig
	if got.InterfaceName != "eth-auto" {
		t.Fatalf("interface=%q, want eth-auto", got.InterfaceName)
	}
	if got.ReadBufferSize != defaultReadBufferSize {
		t.Fatalf("readBufferSize=%d, want %d", got.ReadBufferSize, defaultReadBufferSize)
	}
	if got.EtherType != 0x88b5 {
		t.Fatalf("etherType=%#x, want 0x88b5", got.EtherType)
	}
	if !got.Promiscuous {
		t.Fatal("promiscuous=false, want true")
	}
	if got.WriteBufferWatermark != transportcore.DefaultWriteBufferWatermark() {
		t.Fatalf("watermark=%+v, want %+v", got.WriteBufferWatermark, transportcore.DefaultWriteBufferWatermark())
	}
}

func TestServerBootstrapBindsL2Endpoint(t *testing.T) {
	driver := newFakeDriver()
	group := newL2TestGroup(t)
	initialized := make(chan channel.Channel, 1)

	server, err := bootstrap.NewServerBootstrap().
		Group(group, group).
		Transport(NewTransport(Config{Driver: driver})).
		ChildInitializer(func(ch channel.Channel) error {
			initialized <- ch
			return nil
		}).
		Bind("eth-bind")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	if server.Addr() != "eth-test" {
		t.Fatalf("addr=%q, want eth-test", server.Addr())
	}
	if driver.lastConfig.InterfaceName != "eth-bind" {
		t.Fatalf("interface=%q, want eth-bind", driver.lastConfig.InterfaceName)
	}
	select {
	case ch := <-initialized:
		if ch == nil {
			t.Fatal("initialized channel is nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for child initializer")
	}
}

func TestBindReturnsUnsupportedWhenNativeDriverUnavailable(t *testing.T) {
	if DefaultDriverKind() == DriverKindAFPacket {
		t.Skip("Linux native driver can be available depending on CAP_NET_RAW")
	}
	group := newL2TestGroup(t)
	_, err := bootstrap.NewServerBootstrap().
		Group(group, group).
		Transport(NewTransport(Config{})).
		ChildInitializer(func(channel.Channel) error { return nil }).
		Bind("eth0")
	if !errors.Is(err, ErrUnsupportedDriver) {
		t.Fatalf("err=%v, want %v", err, ErrUnsupportedDriver)
	}
}

type frameRecorder struct {
	payloads chan string
}

func newFrameRecorder() *frameRecorder {
	return &frameRecorder{payloads: make(chan string, 1)}
}

func (r *frameRecorder) ChannelRead(_ *channel.HandlerContext, msg any) {
	frame, ok := msg.(Frame)
	if !ok {
		return
	}
	defer frame.Release()
	select {
	case r.payloads <- string(frame.Payload.Bytes()):
	default:
	}
}

func (r *frameRecorder) waitPayload(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-r.payloads:
		if got != want {
			t.Fatalf("payload=%q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for payload %q", want)
	}
}

type fakeDriver struct {
	endpoint   *fakeEndpoint
	lastConfig Config
}

func newFakeDriver() *fakeDriver {
	return &fakeDriver{endpoint: &fakeEndpoint{
		reads:  make(chan []byte, 4),
		writes: make(chan []byte, 4),
	}}
}

func (d *fakeDriver) Open(_ context.Context, cfg Config) (Endpoint, error) {
	d.lastConfig = cfg
	return d.endpoint, nil
}

type fakeEndpoint struct {
	reads  chan []byte
	writes chan []byte
	closed bool
}

func (e *fakeEndpoint) Addr() string {
	return "eth-test"
}

func (e *fakeEndpoint) ReadFrame(ctx context.Context, alloc buffer.Allocator, _ int) (Frame, error) {
	select {
	case data := <-e.reads:
		buf, err := alloc.Acquire(len(data))
		if err != nil {
			return Frame{}, err
		}
		if _, err := buf.WriteBytes(data); err != nil {
			buf.Release()
			return Frame{}, err
		}
		return Frame{Meta: FrameMeta{InterfaceName: "eth-test", EtherType: 0x88b5}, Payload: buf}, nil
	case <-ctx.Done():
		return Frame{}, ctx.Err()
	}
}

func (e *fakeEndpoint) WriteFrame(_ context.Context, frame Frame) error {
	e.writes <- append([]byte(nil), frame.Payload.Bytes()...)
	return nil
}

func (e *fakeEndpoint) Close() error {
	e.closed = true
	return nil
}

func (e *fakeEndpoint) waitWrite(t *testing.T) []byte {
	t.Helper()
	select {
	case data := <-e.writes:
		return data
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for l2 write")
		return nil
	}
}

func newL2TestGroup(t *testing.T) *transportcore.EventLoopGroup {
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

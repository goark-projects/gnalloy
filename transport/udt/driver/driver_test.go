package driver

import (
	"context"
	"errors"
	"testing"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	transportcore "goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/udt"
)

func TestDriverImplementsUDTContract(t *testing.T) {
	var _ udt.Driver = Driver{}
}

func TestDriverRequiresBackend(t *testing.T) {
	_, err := NewDriver(nil).Dial(context.Background(), bootstrap.ClientConfig{}, udt.DefaultConfig())
	if !errors.Is(err, udt.ErrUnsupportedUDT) {
		t.Fatalf("err=%v, want %v", err, udt.ErrUnsupportedUDT)
	}
	if !errors.Is(err, ErrMissingBackend) {
		t.Fatalf("err=%v, want %v", err, ErrMissingBackend)
	}
}

func TestBackendFuncsRejectMissingDial(t *testing.T) {
	_, err := NewDriver(BackendFuncs{}).Dial(context.Background(), bootstrap.ClientConfig{}, udt.DefaultConfig())
	if !errors.Is(err, udt.ErrUnsupportedUDT) {
		t.Fatalf("err=%v, want %v", err, udt.ErrUnsupportedUDT)
	}
	if !errors.Is(err, ErrMissingDial) {
		t.Fatalf("err=%v, want %v", err, ErrMissingDial)
	}
}

func TestBackendFuncsRejectMissingBind(t *testing.T) {
	_, err := NewDriver(BackendFuncs{}).Bind(context.Background(), bootstrap.ServerConfig{}, udt.DefaultConfig())
	if !errors.Is(err, udt.ErrUnsupportedUDT) {
		t.Fatalf("err=%v, want %v", err, udt.ErrUnsupportedUDT)
	}
	if !errors.Is(err, ErrMissingBind) {
		t.Fatalf("err=%v, want %v", err, ErrMissingBind)
	}
}

func TestDriverDelegatesDialWithNormalizedConfig(t *testing.T) {
	group := newTestGroup(t)
	backend := &recordingBackend{}
	ch, err := bootstrap.NewDialer().
		Group(group).
		Transport(udt.NewTransport(udt.Config{Driver: NewDriver(backend), ReadBufferSize: 8192})).
		Dial("127.0.0.1:9000")
	if err != nil {
		t.Fatal(err)
	}
	if ch == nil {
		t.Fatal("channel is nil")
	}
	if backend.dial.Bootstrap.Address != "127.0.0.1:9000" {
		t.Fatalf("address=%q", backend.dial.Bootstrap.Address)
	}
	if backend.dial.UDT.ReadBufferSize != 8192 {
		t.Fatalf("readBufferSize=%d, want 8192", backend.dial.UDT.ReadBufferSize)
	}
	if backend.dial.UDT.Driver != nil {
		t.Fatal("driver self-reference leaked to backend config")
	}
}

func TestDriverDelegatesBindWithNormalizedConfig(t *testing.T) {
	group := newTestGroup(t)
	backend := &recordingBackend{}
	server, err := bootstrap.NewServerBootstrap().
		Group(group, group).
		Transport(udt.NewTransport(udt.Config{Driver: NewDriver(backend)})).
		ChildInitializer(func(channel.Channel) error { return nil }).
		Bind("127.0.0.1:9000")
	if err != nil {
		t.Fatal(err)
	}
	if server == nil {
		t.Fatal("server is nil")
	}
	if backend.bind.Bootstrap.Address != "127.0.0.1:9000" {
		t.Fatalf("address=%q", backend.bind.Bootstrap.Address)
	}
	if backend.bind.UDT.ReadBufferSize != 64*1024 {
		t.Fatalf("readBufferSize=%d, want %d", backend.bind.UDT.ReadBufferSize, 64*1024)
	}
	if backend.bind.UDT.Driver != nil {
		t.Fatal("driver self-reference leaked to backend config")
	}
}

type recordingBackend struct {
	bind BindConfig
	dial DialConfig
}

func (b *recordingBackend) BindUDT(_ context.Context, cfg BindConfig) (bootstrap.Server, error) {
	b.bind = cfg
	return fakeServer{addr: cfg.Bootstrap.Address}, nil
}

func (b *recordingBackend) DialUDT(_ context.Context, cfg DialConfig) (channel.Channel, error) {
	b.dial = cfg
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	cfg.Bootstrap.Apply(ch)
	if err := cfg.Bootstrap.Initializer(ch); err != nil {
		return nil, err
	}
	return ch, nil
}

type fakeServer struct {
	addr string
}

func (s fakeServer) Addr() string { return s.addr }

func (s fakeServer) Close() error { return nil }

func newTestGroup(t *testing.T) *transportcore.EventLoopGroup {
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

package udt

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

func TestDefaultTransportReportsUnsupportedUDT(t *testing.T) {
	group := newUDTTestGroup(t)
	_, err := bootstrap.NewDialer().
		Group(group).
		Transport(NewTransport(Config{})).
		Dial("127.0.0.1:9000")
	if !errors.Is(err, ErrUnsupportedUDT) {
		t.Fatalf("dial err=%v, want %v", err, ErrUnsupportedUDT)
	}

	group = newUDTTestGroup(t)
	_, err = bootstrap.NewServerBootstrap().
		Group(group, group).
		Transport(NewTransport(Config{})).
		ChildInitializer(func(channel.Channel) error { return nil }).
		Bind("127.0.0.1:9000")
	if !errors.Is(err, ErrUnsupportedUDT) {
		t.Fatalf("bind err=%v, want %v", err, ErrUnsupportedUDT)
	}
}

func TestTransportDelegatesToDriverWithNormalizedConfig(t *testing.T) {
	group := newUDTTestGroup(t)
	driver := &fakeUDTDriver{}
	ch, err := bootstrap.NewDialer().
		Group(group).
		Transport(NewTransport(Config{Driver: driver})).
		Dial("127.0.0.1:9000")
	if err != nil {
		t.Fatal(err)
	}
	if ch == nil {
		t.Fatal("channel is nil")
	}
	if driver.client.Address != "127.0.0.1:9000" {
		t.Fatalf("address=%q", driver.client.Address)
	}
	if driver.cfg.ReadBufferSize != defaultReadBufferSize {
		t.Fatalf("readBufferSize=%d, want %d", driver.cfg.ReadBufferSize, defaultReadBufferSize)
	}
	if driver.cfg.WriteBufferWatermark != transportcore.DefaultWriteBufferWatermark() {
		t.Fatalf("watermark=%+v", driver.cfg.WriteBufferWatermark)
	}
}

type fakeUDTDriver struct {
	cfg    Config
	client bootstrap.ClientConfig
}

func (d *fakeUDTDriver) Dial(_ context.Context, cfg bootstrap.ClientConfig, udtCfg Config) (channel.Channel, error) {
	d.cfg = udtCfg
	d.client = cfg
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	cfg.Apply(ch)
	if err := cfg.Initializer(ch); err != nil {
		return nil, err
	}
	return ch, nil
}

func (d *fakeUDTDriver) Bind(_ context.Context, cfg bootstrap.ServerConfig, udtCfg Config) (bootstrap.Server, error) {
	d.cfg = udtCfg
	return fakeUDTServer{addr: cfg.Address}, nil
}

type fakeUDTServer struct {
	addr string
}

func (s fakeUDTServer) Addr() string { return s.addr }

func (s fakeUDTServer) Close() error { return nil }

func newUDTTestGroup(t *testing.T) *transportcore.EventLoopGroup {
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

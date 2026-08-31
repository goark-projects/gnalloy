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
	"goark.dev/gnalloy/transport/rxtx"
)

func TestDriverImplementsRXTXContract(t *testing.T) {
	var _ rxtx.Driver = Driver{}
}

func TestDriverRequiresBackend(t *testing.T) {
	_, err := NewDriver(nil).Dial(context.Background(), bootstrap.ClientConfig{}, rxtx.DefaultConfig())
	if !errors.Is(err, rxtx.ErrUnsupportedRXTX) {
		t.Fatalf("err=%v, want %v", err, rxtx.ErrUnsupportedRXTX)
	}
	if !errors.Is(err, ErrMissingBackend) {
		t.Fatalf("err=%v, want %v", err, ErrMissingBackend)
	}
}

func TestBackendFuncRejectsNilFunction(t *testing.T) {
	var backend BackendFunc
	_, err := NewDriver(backend).Dial(context.Background(), bootstrap.ClientConfig{}, rxtx.DefaultConfig())
	if !errors.Is(err, rxtx.ErrUnsupportedRXTX) {
		t.Fatalf("err=%v, want %v", err, rxtx.ErrUnsupportedRXTX)
	}
	if !errors.Is(err, ErrMissingDial) {
		t.Fatalf("err=%v, want %v", err, ErrMissingDial)
	}
}

func TestDriverDelegatesDialWithNormalizedConfig(t *testing.T) {
	group := newTestGroup(t)
	backend := &recordingBackend{}
	ch, err := bootstrap.NewDialer().
		Group(group).
		Transport(rxtx.NewTransport(rxtx.Config{Driver: NewDriver(backend), BaudRate: 115200})).
		Dial("COM9")
	if err != nil {
		t.Fatal(err)
	}
	if ch == nil {
		t.Fatal("channel is nil")
	}
	if backend.dial.Bootstrap.Address != "COM9" {
		t.Fatalf("address=%q", backend.dial.Bootstrap.Address)
	}
	if backend.dial.Serial.PortName != "COM9" || backend.dial.Serial.BaudRate != 115200 {
		t.Fatalf("serial=%+v", backend.dial.Serial)
	}
	if backend.dial.Serial.Driver != nil {
		t.Fatal("driver self-reference leaked to backend config")
	}
}

type recordingBackend struct {
	dial DialConfig
}

func (b *recordingBackend) DialRXTX(_ context.Context, cfg DialConfig) (channel.Channel, error) {
	b.dial = cfg
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	cfg.Bootstrap.Apply(ch)
	if err := cfg.Bootstrap.Initializer(ch); err != nil {
		return nil, err
	}
	return ch, nil
}

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

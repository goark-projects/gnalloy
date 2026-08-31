package rxtx

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

func TestTransportImplementsClientContract(t *testing.T) {
	var _ bootstrap.ClientTransport = (*Transport)(nil)
}

func TestDefaultTransportReportsUnsupportedRXTX(t *testing.T) {
	group := newRXTXTestGroup(t)
	_, err := bootstrap.NewDialer().
		Group(group).
		Transport(NewTransport(Config{})).
		Dial("COM1")
	if !errors.Is(err, ErrUnsupportedRXTX) {
		t.Fatalf("err=%v, want %v", err, ErrUnsupportedRXTX)
	}
}

func TestTransportReportsUnsupportedForTypedNilDriver(t *testing.T) {
	var driver *fakeRXTXDriver
	group := newRXTXTestGroup(t)
	_, err := bootstrap.NewDialer().
		Group(group).
		Transport(NewTransport(Config{Driver: driver})).
		Dial("COM1")
	if !errors.Is(err, ErrUnsupportedRXTX) {
		t.Fatalf("err=%v, want %v", err, ErrUnsupportedRXTX)
	}
}

func TestTransportDelegatesToDriverWithNormalizedSerialConfig(t *testing.T) {
	group := newRXTXTestGroup(t)
	driver := &fakeRXTXDriver{}
	ch, err := bootstrap.NewDialer().
		Group(group).
		Transport(NewTransport(Config{Driver: driver, BaudRate: 115200})).
		Dial("COM9")
	if err != nil {
		t.Fatal(err)
	}
	if ch == nil {
		t.Fatal("channel is nil")
	}
	if driver.cfg.PortName != "COM9" {
		t.Fatalf("port=%q, want COM9", driver.cfg.PortName)
	}
	if driver.cfg.BaudRate != 115200 {
		t.Fatalf("baud=%d, want 115200", driver.cfg.BaudRate)
	}
	if driver.cfg.DataBits != 8 || driver.cfg.StopBits != StopBitsOne || driver.cfg.Parity != ParityNone {
		t.Fatalf("serial defaults=%+v", driver.cfg)
	}
	if driver.cfg.ReadBufferSize != defaultReadBufferSize {
		t.Fatalf("readBufferSize=%d, want %d", driver.cfg.ReadBufferSize, defaultReadBufferSize)
	}
}

type fakeRXTXDriver struct {
	cfg Config
}

func (d *fakeRXTXDriver) Dial(_ context.Context, cfg bootstrap.ClientConfig, serialCfg Config) (channel.Channel, error) {
	d.cfg = serialCfg
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	cfg.Apply(ch)
	if err := cfg.Initializer(ch); err != nil {
		return nil, err
	}
	return ch, nil
}

func newRXTXTestGroup(t *testing.T) *transportcore.EventLoopGroup {
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

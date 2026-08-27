package npcap

import (
	"context"
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport/l2"
)

func TestDriverRequiresBackend(t *testing.T) {
	_, err := NewDriver(nil, Config{}).Open(context.Background(), l2.Config{InterfaceName: "ethernet0"})
	if !errors.Is(err, l2.ErrUnsupportedDriver) {
		t.Fatalf("err=%v, want %v", err, l2.ErrUnsupportedDriver)
	}
	if !errors.Is(err, ErrMissingBackend) {
		t.Fatalf("err=%v, want %v", err, ErrMissingBackend)
	}
}

func TestDriverNormalizesConfigForBackend(t *testing.T) {
	backend := &recordingBackend{}
	endpoint, err := NewDriver(backend, Config{Immediate: true, BufferSize: 1 << 20}).Open(context.Background(), l2.Config{
		InterfaceName: "ethernet0",
		EtherType:     0x88b5,
		Promiscuous:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint == nil {
		t.Fatal("endpoint is nil")
	}
	got := backend.config
	if got.InterfaceName != "ethernet0" || got.EtherType != 0x88b5 || !got.Promiscuous || !got.Immediate {
		t.Fatalf("config=%+v", got)
	}
	if got.SnapshotLength != defaultSnapshotLength {
		t.Fatalf("snapshot=%d, want %d", got.SnapshotLength, defaultSnapshotLength)
	}
	if got.BufferSize != 1<<20 {
		t.Fatalf("buffer=%d, want %d", got.BufferSize, 1<<20)
	}
}

func TestDriverRejectsInvalidConfig(t *testing.T) {
	_, err := NewDriver(&recordingBackend{}, Config{BufferSize: -1}).Open(context.Background(), l2.Config{InterfaceName: "ethernet0"})
	if !errors.Is(err, l2.ErrInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, l2.ErrInvalidConfig)
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidConfig)
	}
}

type recordingBackend struct {
	config Config
}

func (b *recordingBackend) OpenNpcap(_ context.Context, cfg Config) (l2.Endpoint, error) {
	b.config = cfg
	return fakeEndpoint("npcap"), nil
}

type fakeEndpoint string

func (e fakeEndpoint) Addr() string {
	return string(e)
}

func (e fakeEndpoint) ReadFrame(context.Context, buffer.Allocator, int) (l2.Frame, error) {
	return l2.Frame{}, context.Canceled
}

func (e fakeEndpoint) WriteFrame(context.Context, l2.Frame) error {
	return nil
}

func (e fakeEndpoint) Close() error {
	return nil
}

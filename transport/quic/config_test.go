package quic

import (
	"context"
	"errors"
	"testing"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/poller/memory"
)

func TestConnectionIDCopiesAndCompares(t *testing.T) {
	src := []byte{1, 2, 3, 4}
	cid, err := NewConnectionID(src)
	if err != nil {
		t.Fatal(err)
	}
	src[0] = 9

	if cid.Len() != 4 {
		t.Fatalf("len=%d, want 4", cid.Len())
	}
	if got := cid.AppendTo(nil); string(got) != string([]byte{1, 2, 3, 4}) {
		t.Fatalf("cid=%v", got)
	}
	if cid.String() != "01020304" {
		t.Fatalf("cid string=%s", cid.String())
	}
	if !cid.Equal(MustConnectionID([]byte{1, 2, 3, 4})) {
		t.Fatal("cid should equal same bytes")
	}
}

func TestConnectionIDRejectsOversized(t *testing.T) {
	_, err := NewConnectionID(make([]byte, MaxConnectionIDLength+1))
	if !errors.Is(err, ErrInvalidConnectionID) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidConnectionID)
	}
}

func TestNormalizeConfigValidatesDefaultsAndBounds(t *testing.T) {
	cfg, err := NormalizeConfig(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Versions) != 1 || cfg.Versions[0] != Version1 {
		t.Fatalf("versions=%v", cfg.Versions)
	}
	if cfg.MaxDatagramSize != DefaultMaxDatagramSize {
		t.Fatalf("max datagram=%d", cfg.MaxDatagramSize)
	}

	_, err = NormalizeConfig(Config{Versions: []Version{0xfaceb00c}, MaxDatagramSize: DefaultMaxDatagramSize})
	if !errors.Is(err, ErrInvalidVersion) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidVersion)
	}
	_, err = NormalizeConfig(Config{MaxDatagramSize: MinInitialDatagramSize - 1})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidConfig)
	}
}

func TestPacketValidAndRelease(t *testing.T) {
	buf := buffer.NewHeapBuffer(8)
	_, _ = buf.WriteBytes([]byte("hello"))
	packet := Packet{
		Type:          PacketInitial,
		Version:       Version1,
		DestinationID: MustConnectionID([]byte{1}),
		SourceID:      MustConnectionID([]byte{2}),
		Payload:       buf,
	}
	if !packet.Valid() {
		t.Fatal("packet should be valid")
	}
	packet.Release()
	if buf.RefCnt() != 0 {
		t.Fatalf("ref=%d, want 0", buf.RefCnt())
	}
}

func TestTransportBindReturnsExplicitNotImplemented(t *testing.T) {
	group, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size: 1,
		PollerFactory: func(int) (transport.Poller, error) {
			return memory.New(), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer group.Close()

	_, err = NewTransport(DefaultConfig()).Bind(context.Background(), bootstrap.ServerConfig{
		Address:     "127.0.0.1:4433",
		WorkerGroup: group,
		ChildInitializer: func(ch channel.Channel) error {
			return nil
		},
	})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("err=%v, want %v", err, ErrNotImplemented)
	}
}

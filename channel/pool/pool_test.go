package pool

import (
	"context"
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

func TestPoolGetPutReusesHealthyChannel(t *testing.T) {
	var created int
	p, err := New(Config{
		MaxIdle: 1,
		Factory: func(context.Context) (channel.Channel, error) {
			created++
			ch, _ := newPoolTestChannel(transport.ChannelID(created))
			return ch, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ch1, err := p.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Put(ch1); err != nil {
		t.Fatal(err)
	}
	ch2, err := p.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ch1 != ch2 || created != 1 {
		t.Fatalf("ch1=%v ch2=%v created=%d", ch1, ch2, created)
	}
}

func TestPoolClosesUnhealthyAndExcessIdle(t *testing.T) {
	healthy := true
	p, err := New(Config{
		MaxIdle: 1,
		Factory: func(context.Context) (channel.Channel, error) {
			ch, _ := newPoolTestChannel(1)
			return ch, nil
		},
		HealthCheck: func(channel.Channel) bool {
			return healthy
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch1, sink1 := newPoolTestChannel(1)
	ch2, sink2 := newPoolTestChannel(2)
	if err := p.Put(ch1); err != nil {
		t.Fatal(err)
	}
	if err := p.Put(ch2); err != nil {
		t.Fatal(err)
	}
	if sink2.closes != 1 {
		t.Fatalf("excess closes=%d, want 1", sink2.closes)
	}
	healthy = false
	got, err := p.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got == ch1 {
		t.Fatal("unhealthy idle channel should not be reused")
	}
	if sink1.closes != 1 {
		t.Fatalf("unhealthy closes=%d, want 1", sink1.closes)
	}
}

func TestPoolCloseAndDiscard(t *testing.T) {
	p, err := New(Config{
		MaxIdle: 1,
		Factory: func(context.Context) (channel.Channel, error) {
			ch, _ := newPoolTestChannel(1)
			return ch, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch, sink := newPoolTestChannel(1)
	if err := p.Put(ch); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if sink.closes != 1 {
		t.Fatalf("closes=%d, want 1", sink.closes)
	}
	next, nextSink := newPoolTestChannel(2)
	if err := p.Discard(next); err != nil {
		t.Fatal(err)
	}
	if nextSink.closes != 1 {
		t.Fatalf("discard closes=%d, want 1", nextSink.closes)
	}
	if _, err := p.Get(context.Background()); !errors.Is(err, ErrClosedPool) {
		t.Fatalf("err=%v, want %v", err, ErrClosedPool)
	}
}

type poolTestSink struct {
	closes int
}

func (s *poolTestSink) Write(any) error {
	return nil
}

func (s *poolTestSink) Flush() error {
	return nil
}

func (s *poolTestSink) Close() error {
	s.closes++
	return nil
}

func newPoolTestChannel(id transport.ChannelID) (*channel.LocalChannel, *poolTestSink) {
	sink := &poolTestSink{}
	ch := channel.NewLocalChannel(id, buffer.NewHeapAllocator(), sink)
	return ch, sink
}

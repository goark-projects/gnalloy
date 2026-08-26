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

func TestSimplePoolLifecycleHandler(t *testing.T) {
	lifecycle := &poolLifecycleRecorder{}
	p, err := NewSimple(SimpleConfig{
		MaxIdle: 1,
		Handler: lifecycle,
		Factory: func(context.Context) (channel.Channel, error) {
			ch, _ := newPoolTestChannel(1)
			return ch, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := p.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Put(ch); err != nil {
		t.Fatal(err)
	}
	again, err := p.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ch != again {
		t.Fatal("simple pool should reuse released channel")
	}
	if lifecycle.created != 1 || lifecycle.acquired != 2 || lifecycle.released != 1 {
		t.Fatalf("lifecycle=%+v, want created=1 acquired=2 released=1", lifecycle)
	}
}

func TestChannelPoolMapCreatesOnePoolPerKey(t *testing.T) {
	var created int
	m, err := NewMap(func(key string) (ChannelPool, error) {
		created++
		return NewSimple(SimpleConfig{
			MaxIdle: 1,
			Factory: func(context.Context) (channel.Channel, error) {
				ch, _ := newPoolTestChannel(transport.ChannelID(len(key)))
				return ch, nil
			},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	a, err := m.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	again, err := m.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Get("b")
	if err != nil {
		t.Fatal(err)
	}
	if a != again || a == b || created != 2 || m.Len() != 2 {
		t.Fatalf("a=%p again=%p b=%p created=%d len=%d", a, again, b, created, m.Len())
	}
	if err := m.Remove("a"); err != nil {
		t.Fatal(err)
	}
	if m.Len() != 1 {
		t.Fatalf("len=%d, want 1 after remove", m.Len())
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get("c"); !errors.Is(err, ErrClosedPool) {
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

type poolLifecycleRecorder struct {
	created  int
	acquired int
	released int
}

func (r *poolLifecycleRecorder) ChannelCreated(channel.Channel) error {
	r.created++
	return nil
}

func (r *poolLifecycleRecorder) ChannelAcquired(channel.Channel) error {
	r.acquired++
	return nil
}

func (r *poolLifecycleRecorder) ChannelReleased(channel.Channel) error {
	r.released++
	return nil
}

package transport

import (
	"testing"

	"goark.dev/gnalloy/transport/poller/memory"
)

type testEventHandler struct {
	id     ChannelID
	fd     FDRef
	events []PollEvent
	closed bool
}

func (h *testEventHandler) ID() ChannelID {
	return h.id
}

func (h *testEventHandler) FD() FDRef {
	return h.fd
}

func (h *testEventHandler) HandleEvent(ev PollEvent) {
	h.events = append(h.events, ev)
}

func (h *testEventHandler) Close() error {
	h.closed = true
	return nil
}

func TestEventLoopSubmitTask(t *testing.T) {
	p := memory.New()
	loop, err := NewEventLoop(EventLoopConfig{ID: 1, Poller: p, StartMillis: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	ran := false
	if err := loop.Submit(func() { ran = true }); err != nil {
		t.Fatal(err)
	}
	if err := loop.RunOnce(0); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("task was not executed")
	}
}

func TestEventLoopDispatchesPollEvent(t *testing.T) {
	p := memory.New()
	loop, err := NewEventLoop(EventLoopConfig{ID: 1, Poller: p, StartMillis: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	h := &testEventHandler{id: 7, fd: FDRef{FD: 123}}
	if err := loop.Register(h, ReadyRead); err != nil {
		t.Fatal(err)
	}
	if err := p.Submit(IORequest{Op: OpRead, FD: h.fd, ChannelID: h.id}); err != nil {
		t.Fatal(err)
	}
	if err := loop.RunOnce(0); err != nil {
		t.Fatal(err)
	}
	if len(h.events) != 1 || h.events[0].Op != OpRead {
		t.Fatalf("events=%+v", h.events)
	}
}

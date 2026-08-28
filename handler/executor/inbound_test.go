package executor

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestInboundHandlerOffloadsChannelReadInOrder(t *testing.T) {
	group, err := NewGroup(Config{Size: 2, QueueSize: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer group.Close()

	delegate := &recordingInbound{done: make(chan struct{})}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("business", NewInboundHandler(group, delegate)); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(1)
	ch.Pipeline().FireChannelRead(2)
	ch.Pipeline().FireChannelRead(3)

	select {
	case <-delegate.done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting offloaded reads")
	}
	delegate.mu.Lock()
	defer delegate.mu.Unlock()
	if !reflect.DeepEqual(delegate.values, []int{1, 2, 3}) {
		t.Fatalf("values=%v", delegate.values)
	}
}

func TestInboundHandlerPropagatesRejectedSubmitAndReleasesMessage(t *testing.T) {
	rejected := rejectedExecutor{err: ErrTaskQueueFull}
	msg := &releaseRecorder{}
	exceptions := &exceptionRecorder{done: make(chan struct{})}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("business", NewInboundHandler(rejected, testReadHandler{})); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("exceptions", exceptions); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(msg)

	select {
	case <-exceptions.done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting exception")
	}
	if msg.releases != 1 {
		t.Fatalf("releases=%d", msg.releases)
	}
	if !errors.Is(exceptions.err, ErrTaskQueueFull) {
		t.Fatalf("exception=%v, want %v", exceptions.err, ErrTaskQueueFull)
	}
}

func TestInboundHandlerReleasesByteBufWhenSubmitRejected(t *testing.T) {
	rejected := rejectedExecutor{err: ErrTaskQueueFull}
	exceptions := &exceptionRecorder{done: make(chan struct{})}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("business", NewInboundHandler(rejected, testReadHandler{})); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("exceptions", exceptions); err != nil {
		t.Fatal(err)
	}

	msg := buffer.NewHeapBuffer(4)
	ch.Pipeline().FireChannelRead(msg)

	select {
	case <-exceptions.done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting exception")
	}
	if msg.RefCnt() != 0 {
		t.Fatalf("ref=%d, want released", msg.RefCnt())
	}
}

func TestInboundHandlerRecoversDelegatePanic(t *testing.T) {
	group, err := NewGroup(Config{Size: 1, QueueSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer group.Close()

	exceptions := &exceptionRecorder{done: make(chan struct{})}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("business", NewInboundHandler(group, panicInbound{})); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("exceptions", exceptions); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead("boom")

	select {
	case <-exceptions.done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting panic exception")
	}
	if !errors.Is(exceptions.err, ErrHandlerPanic) {
		t.Fatalf("exception=%v, want %v", exceptions.err, ErrHandlerPanic)
	}
}

type recordingInbound struct {
	mu     sync.Mutex
	values []int
	done   chan struct{}
}

func (h *recordingInbound) ChannelRead(_ *channel.HandlerContext, msg any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.values = append(h.values, msg.(int))
	if len(h.values) == 3 {
		close(h.done)
	}
}

type rejectedExecutor struct {
	err error
}

func (e rejectedExecutor) Submit(Task) error {
	return e.err
}

type testReadHandler struct{}

func (testReadHandler) ChannelRead(*channel.HandlerContext, any) {}

type releaseRecorder struct {
	releases int
}

func (r *releaseRecorder) Release() {
	r.releases++
}

type exceptionRecorder struct {
	err  error
	done chan struct{}
	once sync.Once
}

func (r *exceptionRecorder) ExceptionCaught(_ *channel.HandlerContext, err error) {
	r.err = err
	r.once.Do(func() { close(r.done) })
}

type panicInbound struct{}

func (panicInbound) ChannelRead(*channel.HandlerContext, any) {
	panic("delegate failed")
}

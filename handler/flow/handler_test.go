package flow

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

type readCollector struct {
	messages      []any
	readCompletes int
}

func (c *readCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	c.messages = append(c.messages, msg)
}

func (c *readCollector) ChannelReadComplete(*channel.HandlerContext) {
	c.readCompletes++
}

func (c *readCollector) release() {
	for _, msg := range c.messages {
		releaseMessage(msg)
	}
	c.messages = nil
}

type errorCollector struct {
	errs []error
}

func (c *errorCollector) ExceptionCaught(_ *channel.HandlerContext, err error) {
	c.errs = append(c.errs, err)
}

func TestHandlerQueuesAndDrainsWhilePaused(t *testing.T) {
	handler, err := NewHandler(Config{StartPaused: true, MaxPendingMessages: 4, MaxPendingBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	collector := &readCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("flow", handler); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}
	defer collector.release()

	ch.Pipeline().FireChannelRead("first")
	ch.Pipeline().FireChannelRead("second")
	ch.Pipeline().FireChannelReadComplete()
	if len(collector.messages) != 0 || collector.readCompletes != 0 {
		t.Fatalf("paused delivered messages=%d completes=%d", len(collector.messages), collector.readCompletes)
	}
	if snapshot := handler.Snapshot(); !snapshot.Paused || snapshot.PendingMessages != 2 {
		t.Fatalf("snapshot=%+v, want paused queue of 2", snapshot)
	}

	if err := handler.Resume(); err != nil {
		t.Fatal(err)
	}
	if got := len(collector.messages); got != 2 {
		t.Fatalf("messages=%d, want 2", got)
	}
	if collector.messages[0] != "first" || collector.messages[1] != "second" {
		t.Fatalf("messages=%v", collector.messages)
	}
	if collector.readCompletes != 1 {
		t.Fatalf("read completes=%d, want 1", collector.readCompletes)
	}
	if snapshot := handler.Snapshot(); snapshot.Paused || snapshot.PendingMessages != 0 {
		t.Fatalf("snapshot=%+v, want drained", snapshot)
	}
}

func TestHandlerUsesAutoReadOptionAsPauseSignal(t *testing.T) {
	handler, err := NewHandler(Config{MaxPendingMessages: 4, MaxPendingBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	collector := &readCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	channel.OptionAutoRead.Set(ch.Options(), false)
	if err := ch.Pipeline().AddLast("flow", handler); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}
	defer collector.release()

	ch.Pipeline().FireChannelRead("queued")
	if len(collector.messages) != 0 {
		t.Fatalf("messages=%d, want queued", len(collector.messages))
	}
	if err := handler.Resume(); err != nil {
		t.Fatal(err)
	}
	if len(collector.messages) != 1 || collector.messages[0] != "queued" {
		t.Fatalf("messages=%v", collector.messages)
	}
	if !channel.OptionAutoRead.Get(ch.Options()) {
		t.Fatal("resume should restore auto-read")
	}
}

func TestHandlerPauseBeforeAddDisablesAutoRead(t *testing.T) {
	handler, err := NewHandler(Config{MaxPendingMessages: 4, MaxPendingBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	handler.Pause()
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("flow", handler); err != nil {
		t.Fatal(err)
	}
	if channel.OptionAutoRead.Get(ch.Options()) {
		t.Fatal("pre-add pause should disable auto-read")
	}
}

func TestHandlerRejectsQueueOverflowAndReleasesMessage(t *testing.T) {
	handler, err := NewHandler(Config{StartPaused: true, MaxPendingMessages: 1, MaxPendingBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	errorsSeen := &errorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("flow", handler); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("errors", errorsSeen); err != nil {
		t.Fatal(err)
	}

	first := testBuffer(t, "one")
	ch.Pipeline().FireChannelRead(first)
	overflow := testBuffer(t, "two")
	ch.Pipeline().FireChannelRead(overflow)
	if overflow.RefCnt() != 0 {
		t.Fatalf("overflow ref=%d, want released", overflow.RefCnt())
	}
	if len(errorsSeen.errs) != 1 || !errors.Is(errorsSeen.errs[0], ErrQueueFull) {
		t.Fatalf("errs=%v, want %v", errorsSeen.errs, ErrQueueFull)
	}
	if snapshot := handler.Snapshot(); snapshot.PendingMessages != 1 || snapshot.DroppedMessages != 1 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	handler.closePending()
}

func TestHandlerReleasesPendingOnInactive(t *testing.T) {
	handler, err := NewHandler(Config{StartPaused: true, MaxPendingMessages: 4, MaxPendingBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	collector := &readCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("flow", handler); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	msg := testBuffer(t, "held")
	ch.Pipeline().FireChannelRead(msg)
	ch.Pipeline().FireChannelInactive()
	if msg.RefCnt() != 0 {
		t.Fatalf("pending ref=%d, want released", msg.RefCnt())
	}
	if len(collector.messages) != 0 {
		t.Fatalf("messages=%d, want none", len(collector.messages))
	}
}

func TestNewHandlerRejectsInvalidConfig(t *testing.T) {
	if _, err := NewHandler(Config{MaxPendingMessages: -1}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("messages err=%v, want %v", err, ErrInvalidConfig)
	}
	if _, err := NewHandler(Config{MaxPendingBytes: -1}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("bytes err=%v, want %v", err, ErrInvalidConfig)
	}
}

func testBuffer(t *testing.T, text string) buffer.ByteBuf {
	t.Helper()
	buf := buffer.NewHeapBuffer(len(text))
	if _, err := buf.WriteBytes([]byte(text)); err != nil {
		t.Fatal(err)
	}
	return buf
}

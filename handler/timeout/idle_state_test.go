package timeout

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/timer"
)

type eventCollector struct {
	events []IdleStateEvent
	errs   []error
	closed bool
}

func (c *eventCollector) UserEventTriggered(_ *channel.HandlerContext, event any) {
	if idle, ok := event.(IdleStateEvent); ok {
		c.events = append(c.events, idle)
	}
}

func (c *eventCollector) ExceptionCaught(_ *channel.HandlerContext, err error) {
	c.errs = append(c.errs, err)
}

type closeSink struct {
	closed bool
}

func (s *closeSink) Write(any) error { return nil }
func (s *closeSink) Flush() error    { return nil }
func (s *closeSink) Close() error {
	s.closed = true
	return nil
}

func TestIdleStateHandlerFiresReaderIdle(t *testing.T) {
	wheel, err := timer.NewWheel(10, 16, 0)
	if err != nil {
		t.Fatal(err)
	}
	collector := &eventCollector{}
	ch := channel.NewLocalChannelWithTimer(1, buffer.NewHeapAllocator(), &closeSink{}, wheel)
	_ = ch.Pipeline().AddLast("idle", NewIdleStateHandler(20, 0, 0))
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelActive()
	wheel.Advance(20, 0)
	if len(collector.events) != 1 || collector.events[0].State != ReaderIdle || !collector.events[0].First {
		t.Fatalf("events=%+v", collector.events)
	}
	wheel.Advance(40, 0)
	if len(collector.events) != 2 || collector.events[1].First {
		t.Fatalf("events=%+v", collector.events)
	}
}

func TestIdleStateHandlerReschedulesOnRead(t *testing.T) {
	wheel, _ := timer.NewWheel(10, 16, 0)
	collector := &eventCollector{}
	ch := channel.NewLocalChannelWithTimer(1, buffer.NewHeapAllocator(), &closeSink{}, wheel)
	_ = ch.Pipeline().AddLast("idle", NewIdleStateHandler(20, 0, 0))
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelActive()
	wheel.Advance(10, 0)
	ch.Pipeline().FireChannelRead(buffer.NewHeapBuffer(1))
	wheel.Advance(20, 0)
	if len(collector.events) != 0 {
		t.Fatalf("events=%+v", collector.events)
	}
	wheel.Advance(30, 0)
	if len(collector.events) != 1 {
		t.Fatalf("events=%+v", collector.events)
	}
}

func TestReadTimeoutHandlerClosesChannel(t *testing.T) {
	wheel, _ := timer.NewWheel(10, 16, 0)
	sink := &closeSink{}
	collector := &eventCollector{}
	ch := channel.NewLocalChannelWithTimer(1, buffer.NewHeapAllocator(), sink, wheel)
	_ = ch.Pipeline().AddLast("timeout", NewReadTimeoutHandler(20))
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelActive()
	wheel.Advance(20, 0)
	if !sink.closed {
		t.Fatal("channel should be closed")
	}
	if len(collector.errs) != 1 || collector.errs[0] != ErrReadTimeout {
		t.Fatalf("errs=%v", collector.errs)
	}
}

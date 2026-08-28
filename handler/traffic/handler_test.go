package traffic

import (
	"reflect"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/timer"
)

func TestHandlerDelaysInboundReadWithChannelTimer(t *testing.T) {
	var now int64
	wheel, err := timer.NewWheel(10, 128, 0)
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannelWithTimer(1, buffer.NewHeapAllocator(), nil, wheel)
	handler, err := NewChannelHandler(Config{
		ReadLimitBytesPerSecond: 4,
		Clock:                   func() int64 { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	capture := &readCapture{}
	if err := ch.Pipeline().AddLast("traffic", handler); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("capture", capture); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(testBuffer(t, "abcd"))
	ch.Pipeline().FireChannelRead(testBuffer(t, "efgh"))
	if !reflect.DeepEqual(capture.payloads, []string{"abcd"}) {
		t.Fatalf("payloads=%v", capture.payloads)
	}
	stats := handler.Stats()
	if stats.ReadBytes != 8 || stats.DelayedReads != 1 {
		t.Fatalf("stats=%+v", stats)
	}

	now = 999
	wheel.Advance(now, 0)
	if !reflect.DeepEqual(capture.payloads, []string{"abcd"}) {
		t.Fatalf("payloads before due=%v", capture.payloads)
	}
	now = 1000
	wheel.Advance(now, 0)
	if !reflect.DeepEqual(capture.payloads, []string{"abcd", "efgh"}) {
		t.Fatalf("payloads after due=%v", capture.payloads)
	}
}

func TestHandlerReleasesDelayedByteBufWhenTimerMissing(t *testing.T) {
	var now int64
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	handler, err := NewChannelHandler(Config{
		ReadLimitBytesPerSecond: 4,
		Clock:                   func() int64 { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("traffic", handler); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(testBuffer(t, "abcd"))
	delayed := testBuffer(t, "efgh")
	ch.Pipeline().FireChannelRead(delayed)
	if delayed.RefCnt() != 0 {
		t.Fatalf("ref=%d, want released", delayed.RefCnt())
	}
}

func TestHandlerQueuesOutboundWriteUntilFlushDelayExpires(t *testing.T) {
	var now int64
	wheel, err := timer.NewWheel(10, 128, 0)
	if err != nil {
		t.Fatal(err)
	}
	sink := &recordingSink{}
	ch := channel.NewLocalChannelWithTimer(1, buffer.NewHeapAllocator(), sink, wheel)
	handler, err := NewChannelHandler(Config{
		WriteLimitBytesPerSecond: 4,
		Clock:                    func() int64 { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("traffic", handler); err != nil {
		t.Fatal(err)
	}

	if err := ch.WriteAndFlush(testBuffer(t, "abcd")); err != nil {
		t.Fatal(err)
	}
	if err := ch.WriteAndFlush(testBuffer(t, "efgh")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sink.writes, []string{"abcd"}) {
		t.Fatalf("writes=%v", sink.writes)
	}
	if sink.flushes != 1 {
		t.Fatalf("flushes=%d", sink.flushes)
	}
	stats := handler.Stats()
	if stats.WrittenBytes != 8 || stats.DelayedWrites != 1 || stats.PendingWrites != 1 || stats.PendingWriteBytes != 4 {
		t.Fatalf("stats=%+v", stats)
	}

	now = 1000
	wheel.Advance(now, 0)
	if !reflect.DeepEqual(sink.writes, []string{"abcd", "efgh"}) {
		t.Fatalf("writes=%v", sink.writes)
	}
	if sink.flushes != 2 {
		t.Fatalf("flushes=%d", sink.flushes)
	}
	stats = handler.Stats()
	if stats.PendingWrites != 0 || stats.PendingWriteBytes != 0 {
		t.Fatalf("stats after drain=%+v", stats)
	}
}

func TestHandlerSharesGlobalControllerAcrossChannels(t *testing.T) {
	var now int64
	controller, err := NewController(Config{
		ReadLimitBytesPerSecond: 4,
		Clock:                   func() int64 { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	wheelA, _ := timer.NewWheel(10, 128, 0)
	wheelB, _ := timer.NewWheel(10, 128, 0)
	chA := channel.NewLocalChannelWithTimer(1, buffer.NewHeapAllocator(), nil, wheelA)
	chB := channel.NewLocalChannelWithTimer(2, buffer.NewHeapAllocator(), nil, wheelB)
	captureA := &readCapture{}
	captureB := &readCapture{}
	if err := chA.Pipeline().AddLast("traffic", NewHandler(controller)); err != nil {
		t.Fatal(err)
	}
	if err := chA.Pipeline().AddLast("capture", captureA); err != nil {
		t.Fatal(err)
	}
	if err := chB.Pipeline().AddLast("traffic", NewHandler(controller)); err != nil {
		t.Fatal(err)
	}
	if err := chB.Pipeline().AddLast("capture", captureB); err != nil {
		t.Fatal(err)
	}

	chA.Pipeline().FireChannelRead(testBuffer(t, "abcd"))
	chB.Pipeline().FireChannelRead(testBuffer(t, "efgh"))
	if len(captureA.payloads) != 1 || len(captureB.payloads) != 0 {
		t.Fatalf("captureA=%v captureB=%v", captureA.payloads, captureB.payloads)
	}
	if stats := controller.Stats(); stats.ReadBytes != 8 || stats.DelayedReads != 1 {
		t.Fatalf("controller stats=%+v", stats)
	}

	now = 1000
	wheelB.Advance(now, 0)
	if !reflect.DeepEqual(captureB.payloads, []string{"efgh"}) {
		t.Fatalf("captureB=%v", captureB.payloads)
	}
}

type readCapture struct {
	payloads []string
}

func (c *readCapture) ChannelRead(_ *channel.HandlerContext, msg any) {
	buf := msg.(buffer.ByteBuf)
	c.payloads = append(c.payloads, string(buf.Bytes()))
	buf.Release()
}

type recordingSink struct {
	writes  []string
	flushes int
}

func (s *recordingSink) Write(msg any) error {
	buf := msg.(buffer.ByteBuf)
	s.writes = append(s.writes, string(buf.Bytes()))
	buf.Release()
	return nil
}

func (s *recordingSink) Flush() error {
	s.flushes++
	return nil
}

func (s *recordingSink) Close() error {
	return nil
}

func testBuffer(t *testing.T, payload string) buffer.ByteBuf {
	t.Helper()
	buf := buffer.NewHeapBuffer(len(payload))
	if _, err := buf.WriteBytes([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	return buf
}

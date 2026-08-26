package flush

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/timer"
)

func TestConsolidationHandlerFlushesOnceAfterReadComplete(t *testing.T) {
	sink := &flushSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	handler, err := NewConsolidationHandler(Config{ExplicitFlushAfterFlushes: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("flush", handler); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead("msg")
	first := ch.FlushFuture()
	second := ch.FlushFuture()
	if sink.flushes != 0 {
		t.Fatalf("flushes=%d before read complete", sink.flushes)
	}
	if first.IsDone() || second.IsDone() {
		t.Fatal("pending futures completed before consolidated flush")
	}

	ch.Pipeline().FireChannelReadComplete()
	if sink.flushes != 1 {
		t.Fatalf("flushes=%d after read complete", sink.flushes)
	}
	if err := first.Await(); err != nil {
		t.Fatal(err)
	}
	if err := second.Await(); err != nil {
		t.Fatal(err)
	}
	stats := handler.Stats()
	if stats.DownstreamFlushes != 1 || stats.ConsolidatedFlushes != 1 || stats.PendingFlushes != 0 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestConsolidationHandlerFlushesWhenExplicitThresholdReached(t *testing.T) {
	sink := &flushSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	handler, err := NewConsolidationHandler(Config{ExplicitFlushAfterFlushes: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("flush", handler); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead("msg")
	if err := ch.Flush(); err != nil {
		t.Fatal(err)
	}
	if sink.flushes != 0 {
		t.Fatalf("flushes=%d after first pending flush", sink.flushes)
	}
	if err := ch.Flush(); err != nil {
		t.Fatal(err)
	}
	if sink.flushes != 1 {
		t.Fatalf("flushes=%d after threshold flush", sink.flushes)
	}
	ch.Pipeline().FireChannelReadComplete()
	if sink.flushes != 1 {
		t.Fatalf("flushes=%d after empty read complete", sink.flushes)
	}
}

func TestConsolidationHandlerSchedulesFlushWhenNoReadInProgress(t *testing.T) {
	wheel, err := timer.NewWheel(10, 64, 0)
	if err != nil {
		t.Fatal(err)
	}
	sink := &flushSink{}
	ch := channel.NewLocalChannelWithTimer(1, buffer.NewHeapAllocator(), sink, wheel)
	handler, err := NewConsolidationHandler(Config{
		ExplicitFlushAfterFlushes:         8,
		ConsolidateWhenNoReadInProgress:   true,
		ConsolidateNoReadFlushDelayMillis: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("flush", handler); err != nil {
		t.Fatal(err)
	}

	first := ch.FlushFuture()
	second := ch.FlushFuture()
	if sink.flushes != 0 {
		t.Fatalf("flushes=%d before scheduled flush", sink.flushes)
	}
	wheel.Advance(9, 0)
	if sink.flushes != 0 {
		t.Fatalf("flushes=%d before deadline", sink.flushes)
	}
	wheel.Advance(10, 0)
	if sink.flushes != 1 {
		t.Fatalf("flushes=%d after scheduled flush", sink.flushes)
	}
	if err := first.Await(); err != nil {
		t.Fatal(err)
	}
	if err := second.Await(); err != nil {
		t.Fatal(err)
	}
}

func TestConsolidationHandlerFailsPendingFuturesOnDownstreamError(t *testing.T) {
	flushErr := errors.New("flush failed")
	sink := &flushSink{flushErr: flushErr}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	handler, err := NewConsolidationHandler(Config{ExplicitFlushAfterFlushes: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("flush", handler); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead("msg")
	future := ch.FlushFuture()
	ch.Pipeline().FireChannelReadComplete()
	if err := future.Await(); !errors.Is(err, flushErr) {
		t.Fatalf("err=%v, want %v", err, flushErr)
	}
}

type flushSink struct {
	flushes  int
	closes   int
	flushErr error
}

func (s *flushSink) Write(any) error {
	return nil
}

func (s *flushSink) Flush() error {
	s.flushes++
	return s.flushErr
}

func (s *flushSink) Close() error {
	s.closes++
	return nil
}

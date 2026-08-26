package metrics

import (
	"errors"
	"testing"
	"time"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/observability"
)

func TestChannelMetricsHandlerRecordsInboundOutboundAndLifecycle(t *testing.T) {
	recorder := observability.NewAtomicChannelRecorder()
	sink := &metricsSink{delay: time.Millisecond}
	ch := channel.NewLocalChannel(42, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("metrics", NewChannelMetricsHandler(Config{Recorder: recorder})); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("slow", slowMetricsHandler{}); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRegistered()
	ch.Pipeline().FireChannelActive()
	ch.Pipeline().FireChannelRead(metricsBuffer(t, "hello"))
	ch.Pipeline().FireChannelReadComplete()
	if err := ch.Write(metricsBuffer(t, "out")); err != nil {
		t.Fatal(err)
	}
	if err := ch.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := ch.Close(); err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireChannelInactive()
	ch.Pipeline().FireChannelUnregistered()

	snapshot := recorder.Snapshot()
	if snapshot.RegisteredChannels != 1 || snapshot.ActiveTransitions != 1 || snapshot.ActiveChannels != 0 {
		t.Fatalf("lifecycle=%+v", snapshot)
	}
	if snapshot.InboundMessages != 1 || snapshot.InboundBytes != 5 || snapshot.InboundCompletions != 1 {
		t.Fatalf("inbound=%+v", snapshot)
	}
	if snapshot.OutboundMessages != 1 || snapshot.OutboundBytes != 3 || snapshot.Flushes != 1 || snapshot.Closes != 1 {
		t.Fatalf("outbound=%+v", snapshot)
	}
	if snapshot.InboundReadNanos == 0 || snapshot.OutboundWriteNanos == 0 || snapshot.FlushNanos == 0 || snapshot.CloseNanos == 0 {
		t.Fatalf("latency not recorded: %+v", snapshot)
	}
	if len(sink.writes) != 1 || string(sink.writes[0].Bytes()) != "out" || sink.flushes != 1 || sink.closes != 1 {
		t.Fatalf("sink writes=%d flushes=%d closes=%d", len(sink.writes), sink.flushes, sink.closes)
	}
	sink.release()
}

func TestChannelMetricsHandlerRecordsErrorsAndCustomSizer(t *testing.T) {
	recorder := observability.NewAtomicChannelRecorder()
	sink := &metricsSink{writeErr: errors.New("write failed")}
	ch := channel.NewLocalChannel(7, buffer.NewHeapAllocator(), sink)
	handler := NewChannelMetricsHandler(Config{
		Recorder: recorder,
		Sizer: observability.MessageSizerFunc(func(any) int64 {
			return 9
		}),
	})
	if err := ch.Pipeline().AddLast("metrics", handler); err != nil {
		t.Fatal(err)
	}

	msg := metricsBuffer(t, "x")
	err := ch.Write(msg)
	if !errors.Is(err, sink.writeErr) {
		t.Fatalf("err=%v, want %v", err, sink.writeErr)
	}
	msg.Release()
	ch.Pipeline().FireExceptionCaught(errors.New("decode failed"))

	snapshot := recorder.Snapshot()
	if snapshot.OutboundMessages != 1 || snapshot.OutboundBytes != 9 || snapshot.Exceptions != 2 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}

type metricsSink struct {
	writes   []buffer.ByteBuf
	flushes  int
	closes   int
	writeErr error
	delay    time.Duration
}

func (s *metricsSink) Write(msg any) error {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if s.writeErr != nil {
		return s.writeErr
	}
	if buf, ok := msg.(buffer.ByteBuf); ok {
		s.writes = append(s.writes, buf)
	}
	return nil
}

func (s *metricsSink) Flush() error {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	s.flushes++
	return nil
}

func (s *metricsSink) Close() error {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	s.closes++
	return nil
}

func (s *metricsSink) release() {
	for _, buf := range s.writes {
		buf.Release()
	}
}

func metricsBuffer(t *testing.T, value string) buffer.ByteBuf {
	t.Helper()
	buf := buffer.NewHeapBuffer(len(value))
	if _, err := buf.WriteBytes([]byte(value)); err != nil {
		buf.Release()
		t.Fatal(err)
	}
	return buf
}

type slowMetricsHandler struct{}

func (slowMetricsHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	time.Sleep(time.Millisecond)
	ctx.FireChannelRead(msg)
}

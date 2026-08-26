package logging

import (
	"context"
	"log/slog"
	"reflect"
	"sync"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestHandlerLogsAndForwardsPipelineEvents(t *testing.T) {
	capture := &slogCapture{}
	logger := slog.New(capture)
	sink := &loggingSink{}
	ch := channel.NewLocalChannel(42, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("logging", NewHandler(Config{Logger: logger})); err != nil {
		t.Fatal(err)
	}
	reads := &loggingReadCapture{}
	if err := ch.Pipeline().AddLast("reads", reads); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelActive()
	ch.Pipeline().FireChannelRead(loggingBuffer(t, "in"))
	ch.Pipeline().FireChannelReadComplete()
	if err := ch.Write(loggingBuffer(t, "out")); err != nil {
		t.Fatal(err)
	}
	if err := ch.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := ch.Close(); err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireExceptionCaught(assertErr("decode failed"))
	sink.release()

	if !reflect.DeepEqual(reads.payloads, []string{"in"}) {
		t.Fatalf("payloads=%v", reads.payloads)
	}
	if len(sink.writes) != 1 || sink.flushes != 1 || sink.closes != 1 {
		t.Fatalf("sink writes=%d flushes=%d closes=%d", len(sink.writes), sink.flushes, sink.closes)
	}
	want := []string{"channel_active", "channel_read", "channel_read_complete", "write", "flush", "close", "exception"}
	if got := capture.events(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v, want %v", got, want)
	}
}

type slogCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *slogCapture) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *slogCapture) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record.Clone())
	return nil
}

func (h *slogCapture) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *slogCapture) WithGroup(string) slog.Handler {
	return h
}

func (h *slogCapture) events() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	events := make([]string, 0, len(h.records))
	for _, record := range h.records {
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "event" {
				events = append(events, attr.Value.String())
				return false
			}
			return true
		})
	}
	return events
}

type loggingReadCapture struct {
	payloads []string
}

func (c *loggingReadCapture) ChannelRead(_ *channel.HandlerContext, msg any) {
	buf := msg.(buffer.ByteBuf)
	c.payloads = append(c.payloads, string(buf.Bytes()))
	buf.Release()
}

type loggingSink struct {
	writes  []buffer.ByteBuf
	flushes int
	closes  int
}

func (s *loggingSink) Write(msg any) error {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		s.writes = append(s.writes, buf)
	}
	return nil
}

func (s *loggingSink) Flush() error {
	s.flushes++
	return nil
}

func (s *loggingSink) Close() error {
	s.closes++
	return nil
}

func (s *loggingSink) release() {
	for _, write := range s.writes {
		write.Release()
	}
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}

func loggingBuffer(t *testing.T, payload string) buffer.ByteBuf {
	t.Helper()
	buf := buffer.NewHeapBuffer(len(payload))
	if _, err := buf.WriteBytes([]byte(payload)); err != nil {
		buf.Release()
		t.Fatal(err)
	}
	return buf
}

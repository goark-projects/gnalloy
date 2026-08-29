package http2

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestStreamMultiplexerEmitsStreamLifecycleEvents(t *testing.T) {
	mux, err := NewStreamMultiplexer(MultiplexerConfig{Server: true})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &streamEventRecorder{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), discardSink{})
	if err := ch.Pipeline().AddLast("mux", mux); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("recorder", recorder); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(HeadersFrame{StreamID: 1, HeaderBlock: testHTTP2Buf(t, "headers")})
	ch.Pipeline().FireChannelRead(DataFrame{StreamID: 1, Flags: FlagEndStream, Data: testHTTP2Buf(t, "body")})

	if len(recorder.events) != 3 {
		t.Fatalf("events=%d, want 3", len(recorder.events))
	}
	if recorder.events[0].Type != StreamEventActive || recorder.events[0].State != StreamOpen {
		t.Fatalf("active event=%+v, want open", recorder.events[0])
	}
	if recorder.events[1].Type != StreamEventRead || recorder.events[1].State != StreamOpen {
		t.Fatalf("headers event=%+v, want read/open", recorder.events[1])
	}
	if recorder.events[2].Type != StreamEventRead || recorder.events[2].State != StreamHalfClosedRemote {
		t.Fatalf("data event=%+v, want read/half-closed-remote", recorder.events[2])
	}
	if mux.ActiveStreams() != 1 {
		t.Fatalf("active streams=%d, want 1", mux.ActiveStreams())
	}
	recorder.release()
}

func TestStreamMultiplexerClosesAfterBothHalfCloses(t *testing.T) {
	mux, err := NewStreamMultiplexer(MultiplexerConfig{Server: true})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &streamEventRecorder{}
	sink := &recordingSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("mux", mux); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("recorder", recorder); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(HeadersFrame{StreamID: 1, HeaderBlock: testHTTP2Buf(t, "request")})
	if err := ch.Write(HeadersFrame{StreamID: 2, Flags: FlagEndStream, HeaderBlock: testHTTP2Buf(t, "push")}); err != nil {
		t.Fatal(err)
	}
	if err := ch.Write(DataFrame{StreamID: 1, Flags: FlagEndStream, Data: testHTTP2Buf(t, "response")}); err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireChannelRead(DataFrame{StreamID: 1, Flags: FlagEndStream, Data: testHTTP2Buf(t, "last")})

	if mux.ActiveStreams() != 1 {
		t.Fatalf("active streams=%d, want only server push stream left", mux.ActiveStreams())
	}
	if last := recorder.events[len(recorder.events)-1]; last.Type != StreamEventClosed || last.StreamID != 1 {
		t.Fatalf("last event=%+v, want stream 1 closed", last)
	}
	sink.release()
	recorder.release()
}

func TestStreamMultiplexerAllowsServerResponseOnClientStream(t *testing.T) {
	mux, err := NewStreamMultiplexer(MultiplexerConfig{Server: true})
	if err != nil {
		t.Fatal(err)
	}
	sink := &recordingSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("mux", mux); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(HeadersBlock{StreamID: 1, Fields: []HeaderField{{Name: ":method", Value: "GET"}}, EndStream: true})
	if err := ch.Write(HeadersBlock{StreamID: 1, Fields: []HeaderField{{Name: ":status", Value: "200"}}}); err != nil {
		t.Fatal(err)
	}
	if err := ch.Write(DataFrame{StreamID: 1, Flags: FlagEndStream, Data: testHTTP2Buf(t, "ok")}); err != nil {
		t.Fatal(err)
	}
	if mux.ActiveStreams() != 0 {
		t.Fatalf("active streams=%d, want 0", mux.ActiveStreams())
	}
	sink.release()
}

func TestStreamMultiplexerRejectsOutboundFlowControlViolation(t *testing.T) {
	mux, err := NewStreamMultiplexer(MultiplexerConfig{
		Server:                  false,
		InitialConnectionWindow: 4,
		InitialStreamWindow:     4,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), discardSink{})
	if err := ch.Pipeline().AddLast("mux", mux); err != nil {
		t.Fatal(err)
	}
	if err := ch.Write(HeadersFrame{StreamID: 1, HeaderBlock: testHTTP2Buf(t, "h")}); err != nil {
		t.Fatal(err)
	}
	if err := ch.Write(DataFrame{StreamID: 1, Data: testHTTP2Buf(t, "12345")}); !errors.Is(err, ErrFlowControl) {
		t.Fatalf("err=%v, want %v", err, ErrFlowControl)
	}
}

func TestStreamMultiplexerAppliesWindowUpdate(t *testing.T) {
	mux, err := NewStreamMultiplexer(MultiplexerConfig{
		Server:                  false,
		InitialConnectionWindow: 4,
		InitialStreamWindow:     4,
	})
	if err != nil {
		t.Fatal(err)
	}
	sink := &recordingSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("mux", mux); err != nil {
		t.Fatal(err)
	}
	if err := ch.Write(HeadersFrame{StreamID: 1, HeaderBlock: testHTTP2Buf(t, "h")}); err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireChannelRead(WindowUpdateFrame{StreamID: 0, Increment: 8})
	ch.Pipeline().FireChannelRead(WindowUpdateFrame{StreamID: 1, Increment: 8})
	if err := ch.Write(DataFrame{StreamID: 1, Data: testHTTP2Buf(t, "12345678")}); err != nil {
		t.Fatal(err)
	}
	sink.release()
}

func TestStreamMultiplexerRejectsWrongLocalStreamParity(t *testing.T) {
	mux, err := NewStreamMultiplexer(MultiplexerConfig{Server: false})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), discardSink{})
	if err := ch.Pipeline().AddLast("mux", mux); err != nil {
		t.Fatal(err)
	}
	if err := ch.Write(HeadersFrame{StreamID: 2, HeaderBlock: testHTTP2Buf(t, "h")}); !errors.Is(err, ErrInvalidStreamID) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidStreamID)
	}
}

func BenchmarkStreamMultiplexerReadData(b *testing.B) {
	mux, err := NewStreamMultiplexer(MultiplexerConfig{Server: true})
	if err != nil {
		b.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), discardSink{})
	recorder := &streamEventRecorder{}
	if err := ch.Pipeline().AddLast("mux", mux); err != nil {
		b.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("recorder", recorder); err != nil {
		b.Fatal(err)
	}
	ch.Pipeline().FireChannelRead(HeadersFrame{StreamID: 1, HeaderBlock: testHTTP2Buf(b, "h")})
	recorder.release()
	data := testHTTP2Buf(b, "abcd")
	defer data.Release()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ch.Pipeline().FireChannelRead(DataFrame{StreamID: 1, Data: data.Retain()})
		recorder.release()
	}
}

type streamEventRecorder struct {
	events []StreamEvent
}

func (r *streamEventRecorder) ChannelRead(_ *channel.HandlerContext, msg any) {
	event, ok := msg.(StreamEvent)
	if !ok {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
		return
	}
	r.events = append(r.events, event)
}

func (r *streamEventRecorder) release() {
	for _, event := range r.events {
		event.Release()
	}
	r.events = r.events[:0]
}

type recordingSink struct {
	messages []any
}

func (s *recordingSink) Write(msg any) error {
	s.messages = append(s.messages, msg)
	return nil
}

func (s *recordingSink) Flush() error { return nil }
func (s *recordingSink) Close() error { return nil }

func (s *recordingSink) release() {
	for _, msg := range s.messages {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
	s.messages = s.messages[:0]
}

type discardSink struct{}

func (discardSink) Write(msg any) error {
	if releasable, ok := msg.(interface{ Release() }); ok {
		releasable.Release()
	}
	return nil
}

func (discardSink) Flush() error { return nil }
func (discardSink) Close() error { return nil }

func testHTTP2Buf(t testing.TB, data string) buffer.ByteBuf {
	t.Helper()
	buf := buffer.NewHeapBuffer(len(data))
	if _, err := buf.WriteBytes([]byte(data)); err != nil {
		buf.Release()
		t.Fatal(err)
	}
	return buf
}

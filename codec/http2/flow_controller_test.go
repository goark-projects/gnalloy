package http2

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestOutboundFlowControllerPassesDataWithinWindow(t *testing.T) {
	sink := &flowSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("flow", NewOutboundFlowController(OutboundFlowControlConfig{
		InitialConnectionWindow: 8,
		InitialStreamWindow:     8,
	})); err != nil {
		t.Fatal(err)
	}

	if err := ch.Write(DataFrame{StreamID: 1, Data: testHTTP2Buf(t, "abcd")}); err != nil {
		t.Fatal(err)
	}

	if len(sink.messages) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.messages))
	}
	ctx, ok := ch.Pipeline().Context("flow")
	if !ok {
		t.Fatal("missing flow context")
	}
	controller := ctx.Handler().(*OutboundFlowController)
	if controller.ConnectionWindow() != 4 || controller.StreamWindow(1) != 4 {
		t.Fatalf("windows conn=%d stream=%d, want 4/4", controller.ConnectionWindow(), controller.StreamWindow(1))
	}
	sink.release()
}

func TestOutboundFlowControllerQueuesDataUntilWindowUpdate(t *testing.T) {
	sink := &flowSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("flow", NewOutboundFlowController(OutboundFlowControlConfig{
		InitialConnectionWindow: 4,
		InitialStreamWindow:     4,
	})); err != nil {
		t.Fatal(err)
	}

	if err := ch.Write(DataFrame{StreamID: 1, Data: testHTTP2Buf(t, "123456")}); err != nil {
		t.Fatal(err)
	}
	if err := ch.Flush(); err != nil {
		t.Fatal(err)
	}
	if len(sink.messages) != 0 {
		t.Fatalf("writes=%d, want queued", len(sink.messages))
	}

	ch.Pipeline().FireChannelRead(WindowUpdateFrame{StreamID: 0, Increment: 2})
	if len(sink.messages) != 0 {
		t.Fatalf("writes=%d, want stream window still blocking", len(sink.messages))
	}
	ch.Pipeline().FireChannelRead(WindowUpdateFrame{StreamID: 1, Increment: 2})

	if len(sink.messages) != 1 {
		t.Fatalf("writes=%d, want released frame", len(sink.messages))
	}
	if sink.flushes != 1 {
		t.Fatalf("flushes=%d, want queued flush to drain once", sink.flushes)
	}
	sink.release()
}

func TestOutboundFlowControllerAppliesInitialWindowSettings(t *testing.T) {
	sink := &flowSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("flow", NewOutboundFlowController(OutboundFlowControlConfig{
		InitialConnectionWindow: 20,
		InitialStreamWindow:     8,
	})); err != nil {
		t.Fatal(err)
	}

	if err := ch.Write(HeadersFrame{StreamID: 1, HeaderBlock: testHTTP2Buf(t, "h")}); err != nil {
		t.Fatal(err)
	}
	if err := ch.Write(DataFrame{StreamID: 1, Data: testHTTP2Buf(t, "123456")}); err != nil {
		t.Fatal(err)
	}
	ctx, ok := ch.Pipeline().Context("flow")
	if !ok {
		t.Fatal("missing flow context")
	}
	controller := ctx.Handler().(*OutboundFlowController)
	ch.Pipeline().FireChannelRead(SettingsFrame{Settings: []Setting{{ID: SettingInitialWindowSize, Value: 4}}})
	if controller.StreamWindow(1) != -2 {
		t.Fatalf("stream window=%d, want -2", controller.StreamWindow(1))
	}

	if err := ch.Write(DataFrame{StreamID: 1, Data: testHTTP2Buf(t, "x")}); err != nil {
		t.Fatal(err)
	}
	if controller.PendingFrames() != 1 {
		t.Fatalf("pending=%d, want 1", controller.PendingFrames())
	}
	ch.Pipeline().FireChannelRead(WindowUpdateFrame{StreamID: 1, Increment: 3})
	if controller.PendingFrames() != 0 || controller.StreamWindow(1) != 0 {
		t.Fatalf("pending=%d stream window=%d, want drained/0", controller.PendingFrames(), controller.StreamWindow(1))
	}
	sink.release()
}

type flowSink struct {
	messages []any
	flushes  int
}

func (s *flowSink) Write(msg any) error {
	s.messages = append(s.messages, msg)
	return nil
}

func (s *flowSink) Flush() error {
	s.flushes++
	return nil
}

func (s *flowSink) Close() error { return nil }

func (s *flowSink) release() {
	for _, msg := range s.messages {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
	s.messages = s.messages[:0]
}

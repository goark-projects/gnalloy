package http3

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestLifecycleHandlerFiresControlEventsAndPropagatesFrames(t *testing.T) {
	events := &http3EventRecorder{}
	inbound := &pipelineInboundCapture{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("lifecycle", NewLifecycleHandler()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("events", events); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("inbound", inbound); err != nil {
		t.Fatal(err)
	}
	defer inbound.release()

	ch.Pipeline().FireChannelRead(SettingsFrame{Settings: []Setting{{ID: 1, Value: 10}}})
	ch.Pipeline().FireChannelRead(GoAwayFrame{ID: 12})

	if len(events.events) != 2 {
		t.Fatalf("events=%d, want 2", len(events.events))
	}
	if events.events[0].Type != LifecycleEventSettings || len(events.events[0].Settings) != 1 {
		t.Fatalf("settings event=%+v", events.events[0])
	}
	if events.events[1].Type != LifecycleEventGoAway || events.events[1].ID != 12 {
		t.Fatalf("goaway event=%+v", events.events[1])
	}
	if len(inbound.messages) != 2 {
		t.Fatalf("inbound=%d, want propagated frames", len(inbound.messages))
	}
}

func TestStatsHandlerCountsInboundAndOutboundFrames(t *testing.T) {
	stats := NewAtomicStatsRecorder()
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("stats", NewStatsHandler(stats)); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	ch.Pipeline().FireChannelRead(DataFrame{Data: testBuf([]byte("abc"))})
	if err := ch.Write(HeadersFrame{HeaderBlock: testBuf([]byte("hp"))}); err != nil {
		t.Fatal(err)
	}

	snapshot := stats.Snapshot()
	if snapshot.InboundFrames != 1 || snapshot.OutboundFrames != 1 {
		t.Fatalf("frame counts=%+v", snapshot)
	}
	if snapshot.InboundDataBytes != 3 || snapshot.OutboundHeaderBytes != 2 {
		t.Fatalf("byte counts=%+v", snapshot)
	}
}

type http3EventRecorder struct {
	events []LifecycleEvent
}

func (r *http3EventRecorder) UserEventTriggered(ctx *channel.HandlerContext, event any) {
	if ev, ok := event.(LifecycleEvent); ok {
		r.events = append(r.events, ev)
	}
	ctx.FireUserEventTriggered(event)
}

func (r *http3EventRecorder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	ctx.FireChannelRead(msg)
}

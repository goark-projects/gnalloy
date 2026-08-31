package http2

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestPrefaceDecoderConsumesFragmentedPrefaceAndForwardsRemainingFrame(t *testing.T) {
	decoder, err := NewFrameDecoder(DefaultMaxFrameSize)
	if err != nil {
		t.Fatal(err)
	}
	events := &prefaceEventRecorder{}
	recorder := &frameRecorder{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("preface", NewPrefaceDecoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("events", events); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("frame", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("recorder", recorder); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(testHTTP2Buf(t, ClientPreface[:8]))
	in, err := ch.Allocator().Acquire(len(ClientPreface[8:]) + FrameHeaderSize)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := in.WriteBytes([]byte(ClientPreface[8:])); err != nil {
		in.Release()
		t.Fatal(err)
	}
	appendRawFrame(t, in, FrameHeader{Type: FrameSettings}, nil)
	ch.Pipeline().FireChannelRead(in)

	if events.prefaces != 1 {
		t.Fatalf("preface events=%d, want 1", events.prefaces)
	}
	frame, ok := recorder.msg.(Frame)
	if !ok {
		t.Fatalf("msg=%T, want Frame", recorder.msg)
	}
	defer frame.Release()
	if frame.Type != FrameSettings || frame.StreamID != 0 {
		t.Fatalf("frame=%+v", frame)
	}
}

func TestPrefaceEncoderWritesClientPrefaceOnActive(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("preface", NewPrefaceEncoder()); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelActive()

	if len(sink.messages) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.messages))
	}
	out := sink.messages[0].(buffer.ByteBuf)
	defer out.Release()
	if string(out.Bytes()) != ClientPreface {
		t.Fatalf("preface=%q", out.Bytes())
	}
}

func TestSettingsAckHandlerAcknowledgesPeerSettingsAndFiresEvent(t *testing.T) {
	sink := &captureSink{}
	events := &prefaceEventRecorder{}
	recorder := &frameRecorder{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("ack", NewSettingsAckHandler()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("events", events); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("recorder", recorder); err != nil {
		t.Fatal(err)
	}

	settings := SettingsFrame{Settings: []Setting{{ID: 1, Value: 4096}}}
	ch.Pipeline().FireChannelRead(settings)

	if len(sink.messages) != 1 {
		t.Fatalf("writes=%d, want settings ack", len(sink.messages))
	}
	ack, ok := sink.messages[0].(SettingsFrame)
	if !ok || !ack.Ack || len(ack.Settings) != 0 {
		t.Fatalf("ack=%+v", sink.messages[0])
	}
	if events.settings != 1 || len(events.lastSettings) != 1 || events.lastSettings[0].Value != 4096 {
		t.Fatalf("settings event count=%d settings=%+v", events.settings, events.lastSettings)
	}
	if got, ok := recorder.msg.(SettingsFrame); !ok || len(got.Settings) != 1 {
		t.Fatalf("msg=%T %+v, want propagated settings", recorder.msg, recorder.msg)
	}
}

func TestSettingsAckHandlerDoesNotAckAck(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("ack", NewSettingsAckHandler()); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(SettingsFrame{Ack: true})

	if len(sink.messages) != 0 {
		t.Fatalf("writes=%d, want no ack loop", len(sink.messages))
	}
}

type prefaceEventRecorder struct {
	prefaces     int
	settings     int
	lastSettings []Setting
}

func (r *prefaceEventRecorder) UserEventTriggered(ctx *channel.HandlerContext, event any) {
	switch ev := event.(type) {
	case PrefaceReceivedEvent:
		r.prefaces++
	case SettingsReceivedEvent:
		r.settings++
		r.lastSettings = ev.Settings
	}
	ctx.FireUserEventTriggered(event)
}

func (r *prefaceEventRecorder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	ctx.FireChannelRead(msg)
}

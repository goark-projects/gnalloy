package tls

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestOptionalHandlerDetectsTLSAndFiresStartEvent(t *testing.T) {
	raw := testClientHelloRecord(t)
	events := &optionalEventRecorder{}
	inbound := &optionalInboundRecorder{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("optional", NewOptionalHandler()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("events", events); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("inbound", inbound); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(optionalBuf(raw[:3]))
	ch.Pipeline().FireChannelRead(optionalBuf(raw[3:]))

	if len(events.optional) != 1 || !events.optional[0].TLS || events.optional[0].ClientHello.ServerName != "api.gnalloy.local" {
		t.Fatalf("optional events=%+v", events.optional)
	}
	if events.starts != 1 {
		t.Fatalf("start events=%d, want 1", events.starts)
	}
	if len(inbound.messages) != 1 || inbound.messages[0].ReadableBytes() != len(raw) {
		t.Fatalf("inbound messages=%d", len(inbound.messages))
	}
	inbound.release()
}

func TestOptionalHandlerPassesPlaintextWithoutStartingTLS(t *testing.T) {
	events := &optionalEventRecorder{}
	inbound := &optionalInboundRecorder{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("optional", NewOptionalHandler()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("events", events); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("inbound", inbound); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(optionalBuf([]byte("GET / HTTP/1.1\r\n\r\n")))

	if len(events.optional) != 1 || events.optional[0].TLS {
		t.Fatalf("optional events=%+v", events.optional)
	}
	if events.starts != 0 {
		t.Fatalf("start events=%d, want 0", events.starts)
	}
	if len(inbound.messages) != 1 || string(inbound.messages[0].Bytes()) != "GET / HTTP/1.1\r\n\r\n" {
		t.Fatalf("inbound=%+v", inbound.messages)
	}
	inbound.release()
}

type optionalEventRecorder struct {
	optional []OptionalEvent
	starts   int
}

func (r *optionalEventRecorder) UserEventTriggered(ctx *channel.HandlerContext, event any) {
	switch ev := event.(type) {
	case OptionalEvent:
		r.optional = append(r.optional, ev)
	case StartEvent:
		r.starts++
	}
	ctx.FireUserEventTriggered(event)
}

func (r *optionalEventRecorder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	ctx.FireChannelRead(msg)
}

type optionalInboundRecorder struct {
	messages []buffer.ByteBuf
}

func (r *optionalInboundRecorder) ChannelRead(_ *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if ok {
		r.messages = append(r.messages, buf)
	}
}

func (r *optionalInboundRecorder) release() {
	for _, msg := range r.messages {
		msg.Release()
	}
}

func optionalBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

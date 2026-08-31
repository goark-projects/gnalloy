package sctp

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/channel/embedded"
)

func TestInboundByteStreamHandlerFiltersByProtocolAndStream(t *testing.T) {
	payload := buffer.NewSharedBuffer([]byte("ok"))
	ch, err := embedded.New(NewInboundByteStreamHandler(7, 3))
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if accepted, err := ch.WriteInbound(NewMessage(7, 3, payload)); err != nil || !accepted {
		t.Fatalf("accepted=%v err=%v, want accepted message", accepted, err)
	}
	msg, ok := ch.ReadInbound()
	if !ok {
		t.Fatal("missing inbound payload")
	}
	got := msg.(buffer.ByteBuf)
	defer got.Release()
	if string(got.Bytes()) != "ok" {
		t.Fatalf("payload=%q, want ok", got.Bytes())
	}
	if payload.RefCnt() != 1 {
		t.Fatalf("payload ref=%d, want caller reference only", payload.RefCnt())
	}

	other := buffer.NewSharedBuffer([]byte("skip"))
	if accepted, err := ch.WriteInbound(NewMessage(8, 3, other)); err != nil || !accepted {
		t.Fatalf("accepted=%v err=%v, want passthrough message", accepted, err)
	}
	passthrough, ok := ch.ReadInbound()
	if !ok {
		t.Fatal("missing passthrough message")
	}
	sctpMsg := passthrough.(Message)
	defer sctpMsg.Release()
	if sctpMsg.ProtocolIdentifier != 8 || string(sctpMsg.Payload.Bytes()) != "skip" {
		t.Fatalf("passthrough=%+v", sctpMsg)
	}
}

func TestOutboundByteStreamHandlerWrapsByteBuf(t *testing.T) {
	payload := buffer.NewSharedBuffer([]byte("hello"))
	ch, err := embedded.New(NewOutboundByteStreamHandler(11, 5))
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if accepted, err := ch.WriteOutbound(payload); err != nil || !accepted {
		t.Fatalf("accepted=%v err=%v, want outbound message", accepted, err)
	}
	msg, ok := ch.ReadOutbound()
	if !ok {
		t.Fatal("missing outbound SCTP message")
	}
	got := msg.(Message)
	defer got.Release()
	if got.ProtocolIdentifier != 11 || got.StreamIdentifier != 5 || !got.Complete {
		t.Fatalf("message=%+v", got)
	}
	if string(got.Payload.Bytes()) != "hello" {
		t.Fatalf("payload=%q, want hello", got.Payload.Bytes())
	}
}

func TestMessageCompletionHandlerAggregatesFragments(t *testing.T) {
	ch, err := embedded.New(NewMessageCompletionHandler())
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	first := NewFragment(2, 1, buffer.NewSharedBuffer([]byte("he")), false)
	second := NewFragment(2, 1, buffer.NewSharedBuffer([]byte("llo")), true)
	if accepted, err := ch.WriteInbound(first); err != nil || accepted {
		t.Fatalf("accepted=%v err=%v, want pending fragment", accepted, err)
	}
	if accepted, err := ch.WriteInbound(second); err != nil || !accepted {
		t.Fatalf("accepted=%v err=%v, want completed message", accepted, err)
	}
	msg, ok := ch.ReadInbound()
	if !ok {
		t.Fatal("missing completed message")
	}
	got := msg.(Message)
	defer got.Release()
	if !got.Complete || string(got.Payload.Bytes()) != "hello" {
		t.Fatalf("message=%+v payload=%q", got, got.Payload.Bytes())
	}
}

func TestMessageCompletionHandlerRejectsContinuationWithoutStart(t *testing.T) {
	errs := &errorCollector{}
	ch, err := embedded.NewWithConfig(embedded.Config{Handlers: []embedded.HandlerSpec{
		{Name: "completion", Handler: NewMessageCompletionHandler()},
		{Name: "errors", Handler: errs},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if accepted, err := ch.WriteInbound(NewFragment(2, 1, buffer.NewSharedBuffer([]byte("tail")), true)); err != nil || accepted {
		t.Fatalf("accepted=%v err=%v, want exception only", accepted, err)
	}
	if !errors.Is(errs.err, ErrMissingFragmentStart) {
		t.Fatalf("err=%v, want %v", errs.err, ErrMissingFragmentStart)
	}
}

type errorCollector struct {
	err error
}

func (c *errorCollector) ExceptionCaught(_ *channel.HandlerContext, err error) {
	c.err = err
}

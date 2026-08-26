package http3

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/channel/embedded"
)

func TestHeaderCodecRoundTripHeadersBlock(t *testing.T) {
	encoder := NewHeaderEncoder()
	outbound, err := embedded.New(encoder)
	if err != nil {
		t.Fatal(err)
	}
	defer outbound.FinishAndReleaseAll()

	if _, err := outbound.WriteOutbound(HeadersBlock{Fields: []HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":path", Value: "/items"},
		{Name: "x-trace", Value: "abc"},
	}}); err != nil {
		t.Fatal(err)
	}
	msg, ok := outbound.ReadOutbound()
	if !ok {
		t.Fatal("missing headers frame")
	}
	frame, ok := msg.(HeadersFrame)
	if !ok {
		t.Fatalf("msg=%T, want HeadersFrame", msg)
	}

	inbound, err := embedded.New(NewHeaderDecoder(HeaderCodecConfig{}))
	if err != nil {
		frame.Release()
		t.Fatal(err)
	}
	defer inbound.FinishAndReleaseAll()
	if _, err := inbound.WriteInbound(frame); err != nil {
		t.Fatal(err)
	}
	decoded, ok := inbound.ReadInbound()
	if !ok {
		t.Fatal("missing decoded headers")
	}
	headers := decoded.(HeadersBlock)
	if len(headers.Fields) != 3 || headers.Fields[2].Name != "x-trace" || headers.Fields[2].Value != "abc" {
		t.Fatalf("headers=%+v", headers)
	}
}

func TestHeaderCodecRoundTripPushPromiseBlock(t *testing.T) {
	encoder := NewHeaderEncoder()
	outbound, err := embedded.New(encoder)
	if err != nil {
		t.Fatal(err)
	}
	defer outbound.FinishAndReleaseAll()

	if _, err := outbound.WriteOutbound(PushPromiseBlock{PushID: 7, Fields: []HeaderField{{Name: ":path", Value: "/style.css"}}}); err != nil {
		t.Fatal(err)
	}
	msg, ok := outbound.ReadOutbound()
	if !ok {
		t.Fatal("missing push promise frame")
	}
	frame, ok := msg.(PushPromiseFrame)
	if !ok {
		t.Fatalf("msg=%T, want PushPromiseFrame", msg)
	}

	inbound, err := embedded.New(NewHeaderDecoder(HeaderCodecConfig{}))
	if err != nil {
		frame.Release()
		t.Fatal(err)
	}
	defer inbound.FinishAndReleaseAll()
	if _, err := inbound.WriteInbound(frame); err != nil {
		t.Fatal(err)
	}
	decoded, ok := inbound.ReadInbound()
	if !ok {
		t.Fatal("missing decoded push promise")
	}
	push := decoded.(PushPromiseBlock)
	if push.PushID != 7 || len(push.Fields) != 1 || push.Fields[0].Value != "/style.css" {
		t.Fatalf("push=%+v", push)
	}
}

func TestHeaderDecoderEnforcesHeaderListSize(t *testing.T) {
	encoder := NewHeaderEncoder()
	outbound, err := embedded.New(encoder)
	if err != nil {
		t.Fatal(err)
	}
	defer outbound.FinishAndReleaseAll()
	if _, err := outbound.WriteOutbound(HeadersBlock{Fields: []HeaderField{{Name: "x-large", Value: "1234567890"}}}); err != nil {
		t.Fatal(err)
	}
	msg, ok := outbound.ReadOutbound()
	if !ok {
		t.Fatal("missing headers frame")
	}

	inbound, err := embedded.New(NewHeaderDecoder(HeaderCodecConfig{MaxHeaderListSize: 8}), http3ExceptionCapture{})
	if err != nil {
		releaseMessage(msg)
		t.Fatal(err)
	}
	defer inbound.FinishAndReleaseAll()
	if _, err := inbound.WriteInbound(msg); err != nil {
		t.Fatal(err)
	}
	got, ok := inbound.ReadInbound()
	if !ok {
		t.Fatal("missing exception")
	}
	if err, ok := got.(error); !ok || !errors.Is(err, ErrHeaderListTooLarge) {
		t.Fatalf("msg=%v, want ErrHeaderListTooLarge", got)
	}
}

type http3ExceptionCapture struct{}

func (http3ExceptionCapture) ExceptionCaught(ctx *channel.HandlerContext, err error) {
	ctx.FireChannelRead(err)
}

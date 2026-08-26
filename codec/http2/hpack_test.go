package http2

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/channel/embedded"
)

func TestHeaderCodecRoundTripWithContinuation(t *testing.T) {
	encoder, err := NewHeaderEncoder(HeaderCodecConfig{MaxFrameSize: 8})
	if err != nil {
		t.Fatal(err)
	}
	outbound, err := embedded.New(encoder)
	if err != nil {
		t.Fatal(err)
	}
	defer outbound.FinishAndReleaseAll()

	wrote, err := outbound.WriteOutbound(HeadersBlock{
		StreamID:  1,
		EndStream: true,
		Fields: []HeaderField{
			{Name: ":method", Value: "GET"},
			{Name: ":path", Value: "/resource"},
			{Name: "x-large", Value: "abcdefghijklmnopqrstuvwxyz"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("headers frame was not emitted")
	}

	decoder, err := NewHeaderDecoder(HeaderCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	inbound, err := embedded.New(decoder)
	if err != nil {
		t.Fatal(err)
	}
	defer inbound.FinishAndReleaseAll()

	frames := 0
	for {
		msg, ok := outbound.ReadOutbound()
		if !ok {
			break
		}
		frames++
		if _, err := inbound.WriteInbound(msg); err != nil {
			t.Fatal(err)
		}
	}
	if frames < 2 {
		t.Fatalf("frames=%d, want continuation split", frames)
	}
	msg, ok := inbound.ReadInbound()
	if !ok {
		t.Fatal("missing decoded headers")
	}
	headers := msg.(HeadersBlock)
	if !headers.EndStream || len(headers.Fields) != 3 || headers.Fields[2].Value != "abcdefghijklmnopqrstuvwxyz" {
		t.Fatalf("headers=%+v", headers)
	}
}

func TestHeaderDecoderRejectsContinuationWithoutHeaders(t *testing.T) {
	decoder, err := NewHeaderDecoder(HeaderCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := embedded.New(decoder, exceptionCapture{})
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	block := buffer.NewHeapBuffer(1)
	if _, err := block.WriteBytes([]byte{0}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteInbound(ContinuationFrame{StreamID: 1, Flags: FlagEndHeaders, HeaderBlock: block}); err != nil {
		t.Fatal(err)
	}
	msg, ok := ch.ReadInbound()
	if !ok {
		t.Fatal("missing exception")
	}
	if err, ok := msg.(error); !ok || !errors.Is(err, ErrHeaderBlock) {
		t.Fatalf("msg=%v, want ErrHeaderBlock", msg)
	}
}

func TestStreamMultiplexerAcceptsDecodedHeadersBlock(t *testing.T) {
	mux, err := NewStreamMultiplexer(MultiplexerConfig{Server: true})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := embedded.New(mux)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if _, err := ch.WriteInbound(HeadersBlock{StreamID: 1, Fields: []HeaderField{{Name: ":method", Value: "GET"}}}); err != nil {
		t.Fatal(err)
	}
	msg, ok := ch.ReadInbound()
	if !ok {
		t.Fatal("missing active event")
	}
	if event := msg.(StreamEvent); event.Type != StreamEventActive || event.StreamID != 1 {
		t.Fatalf("event=%+v", event)
	}
	msg, ok = ch.ReadInbound()
	if !ok {
		t.Fatal("missing read event")
	}
	event := msg.(StreamEvent)
	if event.Type != StreamEventRead {
		t.Fatalf("event=%+v", event)
	}
	if _, ok := event.Frame.(HeadersBlock); !ok {
		t.Fatalf("frame=%T, want HeadersBlock", event.Frame)
	}
}

type exceptionCapture struct{}

func (exceptionCapture) ExceptionCaught(ctx *channel.HandlerContext, err error) {
	ctx.FireChannelRead(err)
}

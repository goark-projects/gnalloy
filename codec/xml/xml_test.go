package xml

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestFrameDecoderEmitsCompleteDocumentOnly(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	decoder, err := NewFrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("xml", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	in := testBuf("<root><item id=\"1\">a</item></root><next/>")
	ch.Pipeline().FireChannelRead(in)
	if len(collector.msgs) != 2 {
		t.Fatalf("msgs=%d, want 2", len(collector.msgs))
	}
	first := collector.msgs[0].(buffer.ByteBuf)
	defer first.Release()
	if got := string(first.Bytes()); got != "<root><item id=\"1\">a</item></root>" {
		t.Fatalf("first=%q", got)
	}
	second := collector.msgs[1].(buffer.ByteBuf)
	defer second.Release()
	if got := string(second.Bytes()); got != "<next/>" {
		t.Fatalf("second=%q", got)
	}
}

func TestTokenDecoderEmitsElementAndTextTokens(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("tokens", NewTokenDecoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(testBuf("<root id=\"1\">text</root>"))
	if len(collector.msgs) != 3 {
		t.Fatalf("msgs=%d, want 3", len(collector.msgs))
	}
	start := collector.msgs[0].(StartElement)
	if start.Name != "root" || len(start.Attrs) != 1 || start.Attrs[0].Value != "1" {
		t.Fatalf("start=%+v", start)
	}
	if text := collector.msgs[1].(CharData); text.Text != "text" {
		t.Fatalf("text=%+v", text)
	}
	if end := collector.msgs[2].(EndElement); end.Name != "root" {
		t.Fatalf("end=%+v", end)
	}
}

type captureInbound struct {
	msgs []any
}

func (h *captureInbound) ChannelRead(_ *channel.HandlerContext, msg any) {
	h.msgs = append(h.msgs, msg)
}

func testBuf(s string) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(s))
	_, _ = buf.WriteBytes([]byte(s))
	return buf
}

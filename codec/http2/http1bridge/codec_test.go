package http1bridge

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel/embedded"
	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/codec/http2"
)

func TestStreamFrameToHTTPObjectCodecServerInboundEndStream(t *testing.T) {
	ch, err := embedded.New(NewStreamFrameToHTTPObjectCodec(Config{Server: true, StreamID: 1}))
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if _, err := ch.WriteInbound(http2.HeadersBlock{
		StreamID: 1,
		Fields: []http2.HeaderField{
			{Name: ":method", Value: "GET"},
			{Name: ":scheme", Value: "https"},
			{Name: ":authority", Value: "example.test"},
			{Name: ":path", Value: "/items"},
		},
		EndStream: true,
	}); err != nil {
		t.Fatal(err)
	}

	req := readInboundAs[http1.Request](t, ch)
	if req.Method != "GET" || req.URI != "/items" || req.Headers.Get("Host") != "example.test" {
		t.Fatalf("request=%+v", req)
	}
	last := readInboundAs[http1.LastHTTPContent](t, ch)
	if last.Data != nil || len(last.Trailers) != 0 {
		t.Fatalf("last=%+v", last)
	}
}

func TestStreamFrameToHTTPObjectCodecClientInboundDataAndTrailers(t *testing.T) {
	ch, err := embedded.New(NewStreamFrameToHTTPObjectCodec(Config{Server: false, StreamID: 2}))
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	body := buffer.NewHeapBuffer(8)
	if _, err := body.WriteBytes([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteInbound(http2.HeadersBlock{
		StreamID: 2,
		Fields: []http2.HeaderField{
			{Name: ":status", Value: "200"},
			{Name: "content-type", Value: "text/plain"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteInbound(http2.DataFrame{StreamID: 2, Data: body}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteInbound(http2.HeadersBlock{
		StreamID:  2,
		Fields:    []http2.HeaderField{{Name: "x-finished", Value: "1"}},
		EndStream: true,
	}); err != nil {
		t.Fatal(err)
	}

	resp := readInboundAs[http1.Response](t, ch)
	if resp.StatusCode != 200 || resp.Headers.Get("content-type") != "text/plain" {
		t.Fatalf("response=%+v", resp)
	}
	content := readInboundAs[http1.HTTPContent](t, ch)
	defer content.Release()
	if got := string(content.Data.Bytes()); got != "ok" {
		t.Fatalf("body=%q", got)
	}
	last := readInboundAs[http1.LastHTTPContent](t, ch)
	if got := last.Trailers.Get("x-finished"); got != "1" {
		t.Fatalf("trailer=%q", got)
	}
}

func TestStreamFrameToHTTPObjectCodecOutboundResponseWithBody(t *testing.T) {
	ch, err := embedded.New(NewStreamFrameToHTTPObjectCodec(Config{Server: true, StreamID: 1}))
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	body := buffer.NewHeapBuffer(8)
	if _, err := body.WriteBytes([]byte("pong")); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteOutbound(http1.Response{
		StatusCode: 200,
		Headers:    http1.Headers{"content-length": "4"},
		Body:       body,
	}); err != nil {
		t.Fatal(err)
	}

	headers := readOutboundAs[http2.HeadersBlock](t, ch)
	if headers.StreamID != 1 || headers.EndStream {
		t.Fatalf("headers=%+v", headers)
	}
	data := readOutboundAs[http2.DataFrame](t, ch)
	defer data.Release()
	if data.StreamID != 1 || data.Flags&http2.FlagEndStream == 0 || string(data.Data.Bytes()) != "pong" {
		t.Fatalf("data=%+v", data)
	}
}

func readInboundAs[T any](t *testing.T, ch *embedded.EmbeddedChannel) T {
	t.Helper()
	msg, ok := ch.ReadInbound()
	if !ok {
		t.Fatal("missing inbound message")
	}
	value, ok := msg.(T)
	if !ok {
		t.Fatalf("message=%T, want requested type", msg)
	}
	return value
}

func readOutboundAs[T any](t *testing.T, ch *embedded.EmbeddedChannel) T {
	t.Helper()
	msg, ok := ch.ReadOutbound()
	if !ok {
		t.Fatal("missing outbound message")
	}
	value, ok := msg.(T)
	if !ok {
		t.Fatalf("message=%T, want requested type", msg)
	}
	return value
}

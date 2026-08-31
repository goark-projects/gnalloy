package http1bridge

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel/embedded"
	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/codec/http3"
)

func TestRequestFromHeadersBlockConvertsPseudoHeaders(t *testing.T) {
	req, err := RequestFromHeadersBlock(http3.HeadersBlock{Fields: []http3.HeaderField{
		{Name: ":method", Value: "POST"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "example.test"},
		{Name: ":path", Value: "/submit"},
		{Name: "x-trace", Value: "a"},
		{Name: "x-trace", Value: "b"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "POST" || req.URI != "/submit" || req.Version != "HTTP/1.1" {
		t.Fatalf("request=%+v", req)
	}
	if got := req.Headers.Get("Host"); got != "example.test" {
		t.Fatalf("host=%q, want example.test", got)
	}
	if got := req.Headers.Get("x-trace"); got != "a, b" {
		t.Fatalf("x-trace=%q, want comma joined", got)
	}
}

func TestRequestFromHeadersBlockRejectsLatePseudoHeader(t *testing.T) {
	_, err := RequestFromHeadersBlock(http3.HeadersBlock{Fields: []http3.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: "x-trace", Value: "a"},
		{Name: ":path", Value: "/late"},
	}})
	if !errors.Is(err, ErrInvalidHeadersBlock) {
		t.Fatalf("err=%v, want ErrInvalidHeadersBlock", err)
	}
}

func TestFrameToHTTPObjectCodecServerInboundObjectStream(t *testing.T) {
	ch, err := embedded.New(NewFrameToHTTPObjectCodec(Config{Server: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	body := buffer.NewHeapBuffer(8)
	if _, err := body.WriteBytes([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteInbound(http3.HeadersBlock{Fields: []http3.HeaderField{
		{Name: ":method", Value: "POST"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "example.test"},
		{Name: ":path", Value: "/items"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteInbound(http3.DataFrame{Data: body}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteInbound(http3.HeadersBlock{Fields: []http3.HeaderField{
		{Name: "x-finished", Value: "1"},
	}}); err != nil {
		t.Fatal(err)
	}

	head := readInboundAs[http1.Request](t, ch)
	if head.Method != "POST" || head.URI != "/items" || head.Headers.Get("Host") != "example.test" {
		t.Fatalf("head=%+v", head)
	}
	content := readInboundAs[http1.HTTPContent](t, ch)
	defer content.Release()
	if got := string(content.Data.Bytes()); got != "hello" {
		t.Fatalf("body=%q, want hello", got)
	}
	last := readInboundAs[http1.LastHTTPContent](t, ch)
	if got := last.Trailers.Get("x-finished"); got != "1" {
		t.Fatalf("trailer=%q, want 1", got)
	}
}

func TestFrameToHTTPObjectCodecClientOutboundObjectStream(t *testing.T) {
	ch, err := embedded.New(NewFrameToHTTPObjectCodec(Config{Server: false}))
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	body := buffer.NewHeapBuffer(8)
	if _, err := body.WriteBytes([]byte("pong")); err != nil {
		t.Fatal(err)
	}
	resp := http1.Response{
		StatusCode: 201,
		Headers: http1.Headers{
			"Content-Type":   "text/plain",
			"Content-Length": "4",
			"Connection":     "close",
		},
		Body: body,
	}
	if _, err := ch.WriteOutbound(resp); err != nil {
		t.Fatal(err)
	}

	headers := readOutboundAs[http3.HeadersBlock](t, ch)
	want := []http3.HeaderField{
		{Name: ":status", Value: "201"},
		{Name: "content-length", Value: "4"},
		{Name: "content-type", Value: "text/plain"},
	}
	if !equalFields(headers.Fields, want) {
		t.Fatalf("headers=%+v, want %+v", headers.Fields, want)
	}
	data := readOutboundAs[http3.DataFrame](t, ch)
	defer data.Release()
	if got := string(data.Data.Bytes()); got != "pong" {
		t.Fatalf("body=%q, want pong", got)
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

func equalFields(got []http3.HeaderField, want []http3.HeaderField) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

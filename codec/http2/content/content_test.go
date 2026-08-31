package content

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel/embedded"
	"goark.dev/gnalloy/codec/http2"
)

func TestDecompressorDecodesGzipDataFrame(t *testing.T) {
	ch, err := embedded.New(NewDecompressor(1024))
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	body := gzipBytes(t, []byte("hello http2"))
	if _, err := ch.WriteInbound(http2.HeadersBlock{
		StreamID: 1,
		Fields: []http2.HeaderField{
			{Name: ":status", Value: "200"},
			{Name: "content-encoding", Value: "gzip"},
			{Name: "content-length", Value: "35"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteInbound(http2.DataFrame{StreamID: 1, Flags: http2.FlagEndStream, Data: testBuf(body)}); err != nil {
		t.Fatal(err)
	}

	headers := readInboundAs[http2.HeadersBlock](t, ch)
	if headerValue(headers.Fields, "content-encoding") != "" || headerValue(headers.Fields, "content-length") != "" {
		t.Fatalf("headers=%+v", headers.Fields)
	}
	data := readInboundAs[http2.DataFrame](t, ch)
	defer data.Release()
	if data.Flags&http2.FlagEndStream == 0 || string(data.Data.Bytes()) != "hello http2" {
		t.Fatalf("data flags=%x body=%q", data.Flags, data.Data.Bytes())
	}
}

func TestCompressorCompressesAcceptedResponseStream(t *testing.T) {
	ch, err := embedded.New(NewResponseCompressor(ResponseCompressorConfig{
		MinBytes: 1,
		Codings:  []Coding{CodingGzip},
	}))
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
			{Name: ":path", Value: "/"},
			{Name: "accept-encoding", Value: "gzip"},
		},
		EndStream: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteOutbound(http2.HeadersBlock{
		StreamID: 1,
		Fields: []http2.HeaderField{
			{Name: ":status", Value: "200"},
			{Name: "content-length", Value: "11"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteOutbound(http2.DataFrame{StreamID: 1, Flags: http2.FlagEndStream, Data: testBuf([]byte("hello http2"))}); err != nil {
		t.Fatal(err)
	}

	headers := readOutboundAs[http2.HeadersBlock](t, ch)
	if got := headerValue(headers.Fields, "content-encoding"); got != "gzip" {
		t.Fatalf("content-encoding=%q", got)
	}
	if headerValue(headers.Fields, "content-length") != "" || headerValue(headers.Fields, "vary") != "accept-encoding" {
		t.Fatalf("headers=%+v", headers.Fields)
	}
	data := readOutboundAs[http2.DataFrame](t, ch)
	defer data.Release()
	if data.Flags&http2.FlagEndStream == 0 {
		t.Fatalf("flags=%x, want END_STREAM", data.Flags)
	}
	if got := gunzipBytes(t, data.Data.Bytes()); string(got) != "hello http2" {
		t.Fatalf("body=%q", got)
	}
}

func gzipBytes(t *testing.T, src []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	if _, err := w.Write(src); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func gunzipBytes(t *testing.T, src []byte) []byte {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func testBuf(src []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(src))
	if len(src) > 0 {
		if _, err := buf.WriteBytes(src); err != nil {
			panic(err)
		}
	}
	return buf
}

func headerValue(fields []http2.HeaderField, name string) string {
	for _, field := range fields {
		if field.Name == name {
			return field.Value
		}
	}
	return ""
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

package http1

import (
	"bytes"
	"compress/gzip"
	"io"
	"strconv"
	"strings"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestContentCompressorCompressesAcceptedResponse(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewResponseEncoder())
	_ = ch.Pipeline().AddLast("compressor", NewContentCompressor(1, ContentCodingGzip))
	defer sink.release()

	ch.Pipeline().FireChannelRead(Request{Headers: Headers{"Accept-Encoding": "br, gzip;q=0.8"}})
	if err := ch.Write(Response{
		StatusCode: 200,
		Headers:    Headers{"Server": "gnalloy"},
		Body:       testBuf([]byte(strings.Repeat("hello", 8))),
	}); err != nil {
		t.Fatal(err)
	}

	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want header and compressed body", len(sink.writes))
	}
	header := string(sink.writes[0].Bytes())
	if !strings.Contains(header, "Content-Encoding: gzip\r\n") || !strings.Contains(header, "Vary: Accept-Encoding\r\n") {
		t.Fatalf("header=%q", header)
	}
	if got := gunzipTestBytes(t, sink.writes[1].Bytes()); got != strings.Repeat("hello", 8) {
		t.Fatalf("body=%q", got)
	}
}

func TestContentDecompressorDecodesGzipResponse(t *testing.T) {
	plain := []byte("hello compressed response")
	compressed := gzipTestBytes(t, plain)
	collector := &responseCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decompressor", NewContentDecompressor(1024))
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(Response{
		StatusCode: 200,
		Headers: Headers{
			"Content-Encoding": "gzip",
			"Content-Length":   strconv.Itoa(len(compressed)),
		},
		Body: testBuf(compressed),
	})

	if len(collector.resps) != 1 {
		t.Fatalf("responses=%d, want 1", len(collector.resps))
	}
	resp := collector.resps[0]
	defer resp.Release()
	if resp.Headers.Get("Content-Encoding") != "" || resp.Headers.Get("Content-Length") != strconv.Itoa(len(plain)) {
		t.Fatalf("headers=%+v", resp.Headers)
	}
	if string(resp.Body.Bytes()) != string(plain) {
		t.Fatalf("body=%q", resp.Body.Bytes())
	}
}

func gzipTestBytes(t *testing.T, src []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(src); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func gunzipTestBytes(t *testing.T, src []byte) string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

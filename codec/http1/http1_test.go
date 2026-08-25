package http1

import (
	"strings"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

type requestCollector struct {
	reqs []Request
}

func (c *requestCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	if req, ok := msg.(Request); ok {
		c.reqs = append(c.reqs, req)
	}
}

type responseCollector struct {
	resps []Response
}

func (c *responseCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	if resp, ok := msg.(Response); ok {
		c.resps = append(c.resps, resp)
	}
}

func TestRequestDecoderWithBody(t *testing.T) {
	decoder, err := NewRequestDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &requestCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte("POST /x HTTP/1.1\r\nContent-Length: 5\r\n\r\nhe")))
	ch.Pipeline().FireChannelRead(testBuf([]byte("llo")))
	if len(collector.reqs) != 1 {
		t.Fatalf("reqs=%d, want 1", len(collector.reqs))
	}
	req := collector.reqs[0]
	defer req.Body.Release()
	if req.Method != "POST" || req.URI != "/x" || string(req.Body.Bytes()) != "hello" {
		t.Fatalf("req=%+v body=%q", req, req.Body.Bytes())
	}
}

func TestResponseDecoderWithBody(t *testing.T) {
	decoder, err := NewResponseDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &responseCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhe")))
	ch.Pipeline().FireChannelRead(testBuf([]byte("llo")))
	if len(collector.resps) != 1 {
		t.Fatalf("resps=%d, want 1", len(collector.resps))
	}
	resp := collector.resps[0]
	defer resp.Release()
	if resp.StatusCode != 200 || resp.Reason != "OK" || string(resp.Body.Bytes()) != "hello" {
		t.Fatalf("resp=%+v body=%q", resp, resp.Body.Bytes())
	}
}

func TestResponseDecoderWithChunkedBody(t *testing.T) {
	decoder, err := NewResponseDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &responseCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n4\r\npo")))
	ch.Pipeline().FireChannelRead(testBuf([]byte("ng\r\n0\r\n\r\n")))
	if len(collector.resps) != 1 {
		t.Fatalf("resps=%d, want 1", len(collector.resps))
	}
	resp := collector.resps[0]
	defer resp.Release()
	if got := string(resp.Body.Bytes()); got != "pong" {
		t.Fatalf("body=%q, want pong", got)
	}
	if resp.Headers.Get("Transfer-Encoding") != "" {
		t.Fatalf("transfer-encoding should be stripped: %+v", resp.Headers)
	}
	if resp.Headers.Get("Content-Length") != "4" {
		t.Fatalf("content-length=%q, want 4", resp.Headers.Get("Content-Length"))
	}
}

func TestRequestEncoder(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewRequestEncoder())
	defer sink.release()

	if err := ch.Write(Request{
		Method:  "POST",
		URI:     "/submit",
		Headers: Headers{"Host": "example.test"},
		Body:    testBuf([]byte("ok")),
	}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	header := string(sink.writes[0].Bytes())
	if header != "POST /submit HTTP/1.1\r\nHost: example.test\r\nContent-Length: 2\r\n\r\n" {
		t.Fatalf("header=%q", header)
	}
	if string(sink.writes[1].Bytes()) != "ok" {
		t.Fatalf("body=%q", sink.writes[1].Bytes())
	}
}

func TestResponseEncoder(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewResponseEncoder())
	defer sink.release()

	if err := ch.Write(Response{StatusCode: 200, Headers: Headers{"Server": "gnalloy"}, Body: testBuf([]byte("ok"))}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	header := string(sink.writes[0].Bytes())
	if header != "HTTP/1.1 200 OK\r\nServer: gnalloy\r\nContent-Length: 2\r\n\r\n" {
		t.Fatalf("header=%q", header)
	}
	if string(sink.writes[1].Bytes()) != "ok" {
		t.Fatalf("body=%q", sink.writes[1].Bytes())
	}
}

func TestRequestDecoderWithChunkedBody(t *testing.T) {
	decoder, err := NewRequestDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &requestCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte("POST /chunk HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nWi")))
	ch.Pipeline().FireChannelRead(testBuf([]byte("ki\r\n5;ext=1\r\npedia\r\n0\r\n\r\n")))
	if len(collector.reqs) != 1 {
		t.Fatalf("reqs=%d, want 1", len(collector.reqs))
	}
	req := collector.reqs[0]
	defer req.Release()
	if req.Method != "POST" || req.URI != "/chunk" {
		t.Fatalf("req=%+v", req)
	}
	if got := string(req.Body.Bytes()); got != "Wikipedia" {
		t.Fatalf("body=%q, want Wikipedia", got)
	}
	if req.Headers.Get("Transfer-Encoding") != "" {
		t.Fatalf("transfer-encoding should be stripped: %+v", req.Headers)
	}
	if req.Headers.Get("Content-Length") != "9" {
		t.Fatalf("content-length=%q, want 9", req.Headers.Get("Content-Length"))
	}
}

func TestResponseEncoderWithChunkedBody(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewResponseEncoder())
	defer sink.release()

	if err := ch.Write(Response{
		StatusCode: 200,
		Headers:    Headers{"Transfer-Encoding": "chunked"},
		Body:       testBuf([]byte("ok")),
	}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 5 {
		t.Fatalf("writes=%d, want 5", len(sink.writes))
	}
	header := string(sink.writes[0].Bytes())
	if !strings.HasPrefix(header, "HTTP/1.1 200 OK\r\n") || !strings.Contains(header, "Transfer-Encoding: chunked\r\n") {
		t.Fatalf("header=%q", header)
	}
	if got := string(sink.writes[1].Bytes()) + string(sink.writes[2].Bytes()) + string(sink.writes[3].Bytes()) + string(sink.writes[4].Bytes()); got != "2\r\nok\r\n0\r\n\r\n" {
		t.Fatalf("chunked body=%q", got)
	}
}

func TestChunkedBodyEncoderWritesStreamingChunks(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewChunkedBodyEncoder())
	defer sink.release()

	if err := ch.Write(Chunk{Data: testBuf([]byte("ab"))}); err != nil {
		t.Fatal(err)
	}
	if err := ch.Write(Chunk{Last: true, Trailers: Headers{"X-Trailer": "v"}}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 4 {
		t.Fatalf("writes=%d, want 4", len(sink.writes))
	}
	got := string(sink.writes[0].Bytes()) + string(sink.writes[1].Bytes()) + string(sink.writes[2].Bytes()) + string(sink.writes[3].Bytes())
	if got != "2\r\nab\r\n0\r\nX-Trailer: v\r\n\r\n" {
		t.Fatalf("encoded=%q", got)
	}
}

func TestContinueHandlerWritesInterimResponseAndPropagatesRequest(t *testing.T) {
	sink := &outboundSink{}
	collector := &requestCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewResponseEncoder())
	_ = ch.Pipeline().AddLast("continue", NewContinueHandler())
	_ = ch.Pipeline().AddLast("collector", collector)
	defer sink.release()

	req := Request{Method: "POST", URI: "/upload", Version: "HTTP/1.1", Headers: Headers{"Expect": "100-continue"}}
	ch.Pipeline().FireChannelRead(req)
	if len(collector.reqs) != 1 {
		t.Fatalf("reqs=%d, want 1", len(collector.reqs))
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	if got := string(sink.writes[0].Bytes()); got != "HTTP/1.1 100 Continue\r\n\r\n" {
		t.Fatalf("continue response=%q", got)
	}
}

type outboundSink struct{ writes []buffer.ByteBuf }

func (s *outboundSink) Write(msg any) error {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		s.writes = append(s.writes, buf)
	}
	return nil
}
func (s *outboundSink) Flush() error { return nil }
func (s *outboundSink) Close() error { return nil }
func (s *outboundSink) release() {
	for _, buf := range s.writes {
		buf.Release()
	}
}

func testBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

package http1

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

type objectCollector struct {
	msgs []any
}

func (c *objectCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	c.msgs = append(c.msgs, msg)
}

func (c *objectCollector) release() {
	for _, msg := range c.msgs {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
}

type objectErrorCollector struct {
	errs []error
}

func (c *objectErrorCollector) ExceptionCaught(_ *channel.HandlerContext, err error) {
	c.errs = append(c.errs, err)
}

func TestRequestObjectDecoderStreamsFixedContent(t *testing.T) {
	decoder, err := NewRequestObjectDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &objectCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)
	defer collector.release()

	ch.Pipeline().FireChannelRead(testBuf([]byte("POST /x HTTP/1.1\r\nContent-Length: 5\r\n\r\nhe")))
	ch.Pipeline().FireChannelRead(testBuf([]byte("llo")))
	if len(collector.msgs) != 3 {
		t.Fatalf("msgs=%d, want 3", len(collector.msgs))
	}
	req := collector.msgs[0].(Request)
	first := collector.msgs[1].(HTTPContent)
	last := collector.msgs[2].(LastHTTPContent)
	if req.Method != "POST" || req.URI != "/x" {
		t.Fatalf("req=%+v", req)
	}
	if string(first.Data.Bytes()) != "he" || string(last.Data.Bytes()) != "llo" {
		t.Fatalf("content=%q last=%q", first.Data.Bytes(), last.Data.Bytes())
	}
}

func TestResponseObjectDecoderStreamsFixedContent(t *testing.T) {
	decoder, err := NewResponseObjectDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &objectCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)
	defer collector.release()

	ch.Pipeline().FireChannelRead(testBuf([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhe")))
	ch.Pipeline().FireChannelRead(testBuf([]byte("llo")))
	if len(collector.msgs) != 3 {
		t.Fatalf("msgs=%d, want 3", len(collector.msgs))
	}
	resp := collector.msgs[0].(Response)
	first := collector.msgs[1].(HTTPContent)
	last := collector.msgs[2].(LastHTTPContent)
	if resp.StatusCode != 200 || resp.Reason != "OK" {
		t.Fatalf("resp=%+v", resp)
	}
	if string(first.Data.Bytes()) != "he" || string(last.Data.Bytes()) != "llo" {
		t.Fatalf("content=%q last=%q", first.Data.Bytes(), last.Data.Bytes())
	}
}

func TestRequestObjectDecoderStreamsChunkedContentAndTrailers(t *testing.T) {
	decoder, err := NewRequestObjectDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &objectCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)
	defer collector.release()

	ch.Pipeline().FireChannelRead(testBuf([]byte("POST /chunk HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n2\r\nab\r\n")))
	ch.Pipeline().FireChannelRead(testBuf([]byte("3\r\ncde\r\n0\r\nX-End: yes\r\n\r\n")))
	if len(collector.msgs) != 4 {
		t.Fatalf("msgs=%d, want 4", len(collector.msgs))
	}
	first := collector.msgs[1].(HTTPContent)
	second := collector.msgs[2].(HTTPContent)
	last := collector.msgs[3].(LastHTTPContent)
	if string(first.Data.Bytes()) != "ab" || string(second.Data.Bytes()) != "cde" {
		t.Fatalf("chunks=%q,%q", first.Data.Bytes(), second.Data.Bytes())
	}
	if last.Trailers.Get("X-End") != "yes" {
		t.Fatalf("trailers=%+v", last.Trailers)
	}
}

func TestResponseObjectDecoderStreamsChunkedContentAndTrailers(t *testing.T) {
	decoder, err := NewResponseObjectDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &objectCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)
	defer collector.release()

	ch.Pipeline().FireChannelRead(testBuf([]byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n2\r\nab\r\n")))
	ch.Pipeline().FireChannelRead(testBuf([]byte("3\r\ncde\r\n0\r\nX-End: yes\r\n\r\n")))
	if len(collector.msgs) != 4 {
		t.Fatalf("msgs=%d, want 4", len(collector.msgs))
	}
	first := collector.msgs[1].(HTTPContent)
	second := collector.msgs[2].(HTTPContent)
	last := collector.msgs[3].(LastHTTPContent)
	if string(first.Data.Bytes()) != "ab" || string(second.Data.Bytes()) != "cde" {
		t.Fatalf("chunks=%q,%q", first.Data.Bytes(), second.Data.Bytes())
	}
	if last.Trailers.Get("X-End") != "yes" {
		t.Fatalf("trailers=%+v", last.Trailers)
	}
}

func TestHTTPObjectAggregatorBuildsFullRequest(t *testing.T) {
	decoder, err := NewRequestObjectDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &requestCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("aggregator", NewHTTPObjectAggregator(1024))
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte("POST /x HTTP/1.1\r\nContent-Length: 5\r\n\r\nhe")))
	ch.Pipeline().FireChannelRead(testBuf([]byte("llo")))
	if len(collector.reqs) != 1 {
		t.Fatalf("reqs=%d, want 1", len(collector.reqs))
	}
	req := collector.reqs[0]
	defer req.Release()
	if req.Method != "POST" || req.URI != "/x" || string(req.Body.Bytes()) != "hello" {
		t.Fatalf("req=%+v body=%q", req, req.Body.Bytes())
	}
}

func TestHTTPObjectAggregatorBuildsFullResponse(t *testing.T) {
	decoder, err := NewResponseObjectDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &responseCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("aggregator", NewHTTPObjectAggregator(1024))
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

func TestHTTPObjectAggregatorRejectsOversizedSingleLastContent(t *testing.T) {
	errorsSeen := &objectErrorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("aggregator", NewHTTPObjectAggregator(4))
	_ = ch.Pipeline().AddLast("errors", errorsSeen)

	ch.Pipeline().FireChannelRead(Request{Method: "POST", URI: "/", Version: "HTTP/1.1", Headers: Headers{}})
	ch.Pipeline().FireChannelRead(LastHTTPContent{Data: testBuf([]byte("hello"))})
	if len(errorsSeen.errs) != 1 {
		t.Fatalf("errs=%d, want 1", len(errorsSeen.errs))
	}
	if !errors.Is(errorsSeen.errs[0], codec.ErrFrameTooLong) {
		t.Fatalf("err=%v, want %v", errorsSeen.errs[0], codec.ErrFrameTooLong)
	}
}

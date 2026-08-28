package cors

import (
	"errors"
	"testing"
	"time"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/http1"
)

func TestNewHandlerRejectsMissingOrigins(t *testing.T) {
	_, err := NewHandler(Config{})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidConfig)
	}
}

func TestHandlerAddsHeadersToSimpleResponse(t *testing.T) {
	sink := &responseSink{}
	h, err := NewHandler(Config{
		AllowedOrigins:   []string{"https://example.com"},
		AllowCredentials: true,
		ExposedHeaders:   []string{"X-Trace"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("cors", h)
	_ = ch.Pipeline().AddLast("app", appResponder{status: 200})

	ch.Pipeline().FireChannelRead(http1.Request{
		Method:  "GET",
		URI:     "/",
		Version: "HTTP/1.1",
		Headers: http1.Headers{headerOrigin: "https://example.com"},
	})
	resp := sink.only(t)
	if resp.Headers.Get(headerAllowOrigin) != "https://example.com" {
		t.Fatalf("allow-origin=%q", resp.Headers.Get(headerAllowOrigin))
	}
	if resp.Headers.Get(headerAllowCredentials) != "true" {
		t.Fatalf("allow-credentials=%q", resp.Headers.Get(headerAllowCredentials))
	}
	if resp.Headers.Get(headerExposeHeaders) != "X-Trace" {
		t.Fatalf("expose=%q", resp.Headers.Get(headerExposeHeaders))
	}
}

func TestHandlerAnswersPreflight(t *testing.T) {
	sink := &responseSink{}
	collector := &requestSink{}
	h, err := NewHandler(Config{
		AllowedOrigins: []string{"https://example.com"},
		AllowedMethods: []string{"GET", "PUT"},
		AllowAnyHeader: true,
		MaxAge:         10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("cors", h)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(http1.Request{
		Method:  "OPTIONS",
		URI:     "/",
		Version: "HTTP/1.1",
		Headers: http1.Headers{
			headerOrigin:         "https://example.com",
			headerRequestMethod:  "PUT",
			headerRequestHeaders: "X-Token",
		},
	})
	if len(collector.requests) != 0 {
		t.Fatalf("requests=%d, want 0", len(collector.requests))
	}
	resp := sink.only(t)
	if resp.StatusCode != preflightStatus {
		t.Fatalf("status=%d, want %d", resp.StatusCode, preflightStatus)
	}
	if resp.Headers.Get(headerAllowMethods) != "GET, PUT" {
		t.Fatalf("allow-methods=%q", resp.Headers.Get(headerAllowMethods))
	}
	if resp.Headers.Get(headerAllowHeaders) != "X-Token" {
		t.Fatalf("allow-headers=%q", resp.Headers.Get(headerAllowHeaders))
	}
	if resp.Headers.Get(headerMaxAge) != "10" {
		t.Fatalf("max-age=%q", resp.Headers.Get(headerMaxAge))
	}
}

func TestHandlerShortCircuitsForbiddenOrigin(t *testing.T) {
	sink := &responseSink{}
	collector := &requestSink{}
	h, err := NewHandler(Config{
		AllowedOrigins: []string{"https://example.com"},
		ShortCircuit:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("cors", h)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(http1.Request{
		Method:  "GET",
		URI:     "/",
		Version: "HTTP/1.1",
		Headers: http1.Headers{headerOrigin: "https://bad.example"},
	})
	if len(collector.requests) != 0 {
		t.Fatalf("requests=%d, want 0", len(collector.requests))
	}
	resp := sink.only(t)
	if resp.StatusCode != forbiddenStatus {
		t.Fatalf("status=%d, want %d", resp.StatusCode, forbiddenStatus)
	}
}

func TestHandlerKeepsResponseOrderForMixedOrigins(t *testing.T) {
	sink := &responseSink{}
	h, err := NewHandler(Config{AllowAnyOrigin: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("cors", h)
	_ = ch.Pipeline().AddLast("app", appResponder{status: 200})

	ch.Pipeline().FireChannelRead(http1.Request{Method: "GET", URI: "/", Version: "HTTP/1.1", Headers: http1.Headers{}})
	ch.Pipeline().FireChannelRead(http1.Request{Method: "GET", URI: "/", Version: "HTTP/1.1", Headers: http1.Headers{headerOrigin: "https://example.com"}})
	if len(sink.responses) != 2 {
		t.Fatalf("responses=%d, want 2", len(sink.responses))
	}
	if got := sink.responses[0].Headers.Get(headerAllowOrigin); got != "" {
		t.Fatalf("first allow-origin=%q, want empty", got)
	}
	if got := sink.responses[1].Headers.Get(headerAllowOrigin); got != "*" {
		t.Fatalf("second allow-origin=%q, want *", got)
	}
}

type appResponder struct {
	status int
}

func (a appResponder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if _, ok := msg.(http1.Request); !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := ctx.Write(http1.Response{StatusCode: a.status, Headers: http1.Headers{}}); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

type requestSink struct {
	requests []http1.Request
}

func (s *requestSink) ChannelRead(_ *channel.HandlerContext, msg any) {
	if req, ok := msg.(http1.Request); ok {
		s.requests = append(s.requests, req)
	}
}

type responseSink struct {
	responses []http1.Response
}

func (s *responseSink) Write(msg any) error {
	resp, ok := msg.(http1.Response)
	if !ok {
		return channel.ErrInvalidMessage
	}
	s.responses = append(s.responses, resp)
	return nil
}

func (s *responseSink) Flush() error {
	return nil
}

func (s *responseSink) Close() error {
	return nil
}

func (s *responseSink) only(t *testing.T) http1.Response {
	t.Helper()
	if len(s.responses) != 1 {
		t.Fatalf("responses=%d, want 1", len(s.responses))
	}
	return s.responses[0]
}

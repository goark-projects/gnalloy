package proxy

import (
	"bytes"
	"errors"
	"net"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestHTTPConnectRequestAndResponse(t *testing.T) {
	req, err := AppendHTTPConnectRequest(nil, HTTPConnectRequest{Target: "example.com:443"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(req, []byte("CONNECT example.com:443 HTTP/1.1\r\n")) || !bytes.Contains(req, []byte("Host: example.com:443\r\n")) {
		t.Fatalf("request=%q", req)
	}
	resp, consumed, err := ParseHTTPConnectResponse([]byte("HTTP/1.1 200 Connection Established\r\nProxy-Agent: test\r\n\r\nextra"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || resp.Reason != "Connection Established" || resp.Headers["Proxy-Agent"] != "test" {
		t.Fatalf("resp=%+v", resp)
	}
	if consumed != len("HTTP/1.1 200 Connection Established\r\nProxy-Agent: test\r\n\r\n") {
		t.Fatalf("consumed=%d", consumed)
	}
}

func TestHTTPConnectClientWritesRequestAndEmitsEvent(t *testing.T) {
	sink := &proxyCaptureSink{}
	events := &proxyEventCapture{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	client, err := NewHTTPConnectClient(HTTPConnectRequest{Target: "example.com:443"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("connect", client); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("events", events); err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireChannelActive()
	if len(sink.writes) != 1 || sink.flushes != 1 {
		t.Fatalf("writes=%d flushes=%d", len(sink.writes), sink.flushes)
	}
	buf := buffer.NewHeapBuffer(64)
	_, _ = buf.WriteBytes([]byte("HTTP/1.1 200 OK\r\n\r\nhello"))
	ch.Pipeline().FireChannelRead(buf)
	if len(events.events) != 1 {
		t.Fatalf("events=%d, want 1", len(events.events))
	}
	if len(events.reads) != 1 {
		t.Fatalf("reads=%d, want leftover data", len(events.reads))
	}
	leftover := events.reads[0].(buffer.ByteBuf)
	if string(leftover.Bytes()) != "hello" {
		t.Fatalf("leftover=%q", leftover.Bytes())
	}
	leftover.Release()
	sink.release()
}

func TestSOCKS5GreetingConnectAndReply(t *testing.T) {
	greeting, err := AppendSOCKS5Greeting(nil, SOCKS5MethodNoAuth)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(greeting, []byte{0x05, 0x01, 0x00}) {
		t.Fatalf("greeting=%v", greeting)
	}
	method, n, err := ParseSOCKS5GreetingResponse([]byte{0x05, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	if method != SOCKS5MethodNoAuth || n != 2 {
		t.Fatalf("method=%d n=%d", method, n)
	}
	connect, err := AppendSOCKS5Connect(nil, "example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x05, 0x01, 0x00, 0x03, 0x0b}; !bytes.Equal(connect[:5], want) {
		t.Fatalf("connect=%v", connect)
	}
	reply, consumed, err := ParseSOCKS5Reply([]byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x01, 0xbb})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Status != 0 || reply.Host != "127.0.0.1" || reply.Port != 443 || consumed != 10 {
		t.Fatalf("reply=%+v consumed=%d", reply, consumed)
	}
}

func TestSOCKS5ClientWritesGreetingConnectAndEmitsEvent(t *testing.T) {
	sink := &proxyCaptureSink{}
	events := &proxyEventCapture{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	client, err := NewSOCKS5Client("example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("socks5", client); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("events", events); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelActive()
	if len(sink.writes) != 1 || sink.flushes != 1 {
		t.Fatalf("writes=%d flushes=%d", len(sink.writes), sink.flushes)
	}
	first := sink.writes[0].(buffer.ByteBuf)
	if !bytes.Equal(first.Bytes(), []byte{0x05, 0x01, 0x00}) {
		t.Fatalf("greeting=%v", first.Bytes())
	}

	ch.Pipeline().FireChannelRead(byteBufWithBytes([]byte{0x05}))
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want no connect before full greeting reply", len(sink.writes))
	}
	ch.Pipeline().FireChannelRead(byteBufWithBytes([]byte{0x00}))
	if len(sink.writes) != 2 || sink.flushes != 2 {
		t.Fatalf("writes=%d flushes=%d, want connect request", len(sink.writes), sink.flushes)
	}
	connect := sink.writes[1].(buffer.ByteBuf)
	if want := []byte{0x05, 0x01, 0x00, 0x03, 0x0b}; !bytes.Equal(connect.Bytes()[:5], want) {
		t.Fatalf("connect=%v", connect.Bytes())
	}

	reply := []byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x01, 0xbb}
	ch.Pipeline().FireChannelRead(byteBufWithBytes(append(reply, []byte("hello")...)))
	if len(events.events) != 1 {
		t.Fatalf("events=%d, want SOCKS5 event", len(events.events))
	}
	event, ok := events.events[0].(SOCKS5Event)
	if !ok || event.Method != SOCKS5MethodNoAuth || event.Reply.Status != 0 {
		t.Fatalf("event=%+v", events.events[0])
	}
	if len(events.reads) != 1 {
		t.Fatalf("reads=%d, want leftover data", len(events.reads))
	}
	leftover := events.reads[0].(buffer.ByteBuf)
	if string(leftover.Bytes()) != "hello" {
		t.Fatalf("leftover=%q", leftover.Bytes())
	}
	leftover.Release()
	sink.release()
}

func TestSOCKS5ClientRejectsHandshakeFailure(t *testing.T) {
	sink := &proxyCaptureSink{}
	errorsSeen := &proxyErrorCapture{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	client, err := NewSOCKS5Client("example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("socks5", client); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("errors", errorsSeen); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelActive()
	ch.Pipeline().FireChannelRead(byteBufWithBytes([]byte{0x05, 0xff}))
	if len(errorsSeen.errors) != 1 || !errors.Is(errorsSeen.errors[0], ErrHandshakeFailed) {
		t.Fatalf("errors=%v, want ErrHandshakeFailed", errorsSeen.errors)
	}
	sink.release()
}

func TestParseHAProxyV1AndV2(t *testing.T) {
	v1 := []byte("PROXY TCP4 192.0.2.1 198.51.100.1 12345 443\r\nGET / HTTP/1.1\r\n")
	info, consumed, err := ParseHAProxyHeader(v1)
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != 1 || info.Protocol != "TCP4" || !info.SourceIP.Equal(net.ParseIP("192.0.2.1")) || info.SourcePort != 12345 {
		t.Fatalf("info=%+v", info)
	}
	if consumed != len("PROXY TCP4 192.0.2.1 198.51.100.1 12345 443\r\n") {
		t.Fatalf("consumed=%d", consumed)
	}

	v2 := append([]byte(nil), haproxyV2Signature...)
	v2 = append(v2, 0x21, 0x11, 0x00, 0x0c)
	v2 = append(v2, []byte{192, 0, 2, 1, 198, 51, 100, 1, 0x30, 0x39, 0x01, 0xbb}...)
	info, consumed, err = ParseHAProxyHeader(v2)
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != 2 || info.Protocol != "TCP4" || !info.DestIP.Equal(net.ParseIP("198.51.100.1")) || info.DestPort != 443 {
		t.Fatalf("info=%+v", info)
	}
	if consumed != len(v2) {
		t.Fatalf("consumed=%d", consumed)
	}
}

type proxyCaptureSink struct {
	writes  []any
	flushes int
}

func (s *proxyCaptureSink) Write(msg any) error {
	s.writes = append(s.writes, msg)
	return nil
}

func (s *proxyCaptureSink) Flush() error {
	s.flushes++
	return nil
}

func (s *proxyCaptureSink) Close() error {
	return nil
}

func (s *proxyCaptureSink) release() {
	for _, msg := range s.writes {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
}

type proxyEventCapture struct {
	events []any
	reads  []any
}

func (h *proxyEventCapture) UserEventTriggered(_ *channel.HandlerContext, event any) {
	h.events = append(h.events, event)
}

func (h *proxyEventCapture) ChannelRead(_ *channel.HandlerContext, msg any) {
	h.reads = append(h.reads, msg)
}

type proxyErrorCapture struct {
	errors []error
}

func (h *proxyErrorCapture) ExceptionCaught(_ *channel.HandlerContext, err error) {
	h.errors = append(h.errors, err)
}

func byteBufWithBytes(src []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(src))
	_, _ = buf.WriteBytes(src)
	return buf
}

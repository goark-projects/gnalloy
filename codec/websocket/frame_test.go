package websocket

import (
	"strings"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/http1"
)

type frameCollector struct {
	frames []Frame
}

func (c *frameCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	if frame, ok := msg.(Frame); ok {
		c.frames = append(c.frames, frame)
	}
}

func TestFrameDecoderMaskedPayload(t *testing.T) {
	decoder, err := NewFrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &frameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte{0x81, 0x82, 1, 2, 3, 4, 'h' ^ 1}))
	ch.Pipeline().FireChannelRead(testBuf([]byte{'i' ^ 2}))
	if len(collector.frames) != 1 {
		t.Fatalf("frames=%d, want 1", len(collector.frames))
	}
	frame := collector.frames[0]
	defer frame.Payload.Release()
	if !frame.Final || frame.Opcode != OpcodeText || string(frame.Payload.Bytes()) != "hi" {
		t.Fatalf("frame=%+v payload=%q", frame, frame.Payload.Bytes())
	}
}

func TestFrameEncoder(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewFrameEncoder())
	defer sink.release()

	if err := ch.Write(Frame{Final: true, Opcode: OpcodeText, Payload: testBuf([]byte("hi"))}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	if string(sink.writes[0].Bytes()) != string([]byte{0x81, 0x02}) || string(sink.writes[1].Bytes()) != "hi" {
		t.Fatalf("writes=%q,%q", sink.writes[0].Bytes(), sink.writes[1].Bytes())
	}
}

func TestAcceptKeyMatchesRFCExample(t *testing.T) {
	got := AcceptKey("dGhlIHNhbXBsZSBub25jZQ==")
	if got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("accept=%q", got)
	}
}

func TestServerHandshakeWritesSwitchingProtocolsAndRemovesHTTPHandlers(t *testing.T) {
	sink := &outboundSink{}
	events := &eventCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", http1.NewResponseEncoder())
	_ = ch.Pipeline().AddLast("httpDecoder", passHandler{})
	_ = ch.Pipeline().AddLast("handshake", NewServerHandshakeHandler("/ws", "httpDecoder", "handshake"))
	_ = ch.Pipeline().AddLast("events", events)
	defer sink.release()

	ch.Pipeline().FireChannelRead(http1.Request{
		Method:  "GET",
		URI:     "/ws",
		Version: "HTTP/1.1",
		Headers: http1.Headers{
			"Host":                  "example.com",
			"Upgrade":               "websocket",
			"Connection":            "keep-alive, Upgrade",
			"Sec-WebSocket-Key":     "dGhlIHNhbXBsZSBub25jZQ==",
			"Sec-WebSocket-Version": "13",
		},
	})
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	header := string(sink.writes[0].Bytes())
	if !strings.HasPrefix(header, "HTTP/1.1 101 Switching Protocols\r\n") ||
		!strings.Contains(header, "Upgrade: websocket\r\n") ||
		!strings.Contains(header, "Connection: Upgrade\r\n") ||
		!strings.Contains(header, "Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n") {
		t.Fatalf("handshake response=%q", header)
	}
	if _, ok := ch.Pipeline().Context("httpDecoder"); ok {
		t.Fatalf("httpDecoder should be removed after upgrade")
	}
	if _, ok := ch.Pipeline().Context("handshake"); ok {
		t.Fatalf("handshake should be removed after upgrade")
	}
	if len(events.events) != 1 {
		t.Fatalf("events=%d, want 1", len(events.events))
	}
}

func TestControlFrameHandlerRespondsToPing(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewFrameEncoder())
	_ = ch.Pipeline().AddLast("control", NewControlFrameHandler())
	defer sink.release()

	ch.Pipeline().FireChannelRead(Frame{Final: true, Opcode: OpcodePing, Payload: testBuf([]byte("x"))})
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	if string(sink.writes[0].Bytes()) != string([]byte{0x8a, 0x01}) || string(sink.writes[1].Bytes()) != "x" {
		t.Fatalf("pong writes=%q,%q", sink.writes[0].Bytes(), sink.writes[1].Bytes())
	}
}

func TestControlFrameHandlerEchoesCloseAndClosesChannel(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewFrameEncoder())
	_ = ch.Pipeline().AddLast("control", NewControlFrameHandler())
	defer sink.release()

	ch.Pipeline().FireChannelRead(Frame{Final: true, Opcode: OpcodeClose, Payload: testBuf([]byte{0x03, 0xe8})})
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	if sink.closes != 1 {
		t.Fatalf("closes=%d, want 1", sink.closes)
	}
	if status, ok := ParseCloseStatus(Frame{Opcode: OpcodeClose, Payload: sink.writes[1]}); !ok || status.Code != 1000 {
		t.Fatalf("close status=%+v ok=%v", status, ok)
	}
}

func TestFragmentAggregatorCombinesTextFragments(t *testing.T) {
	collector := &frameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("aggregator", NewFragmentAggregator(1024))
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(Frame{Final: false, Opcode: OpcodeText, Payload: testBuf([]byte("hel"))})
	ch.Pipeline().FireChannelRead(Frame{Final: true, Opcode: OpcodeContinuation, Payload: testBuf([]byte("lo"))})
	if len(collector.frames) != 1 {
		t.Fatalf("frames=%d, want 1", len(collector.frames))
	}
	frame := collector.frames[0]
	defer frame.Payload.Release()
	if !frame.Final || frame.Opcode != OpcodeText || string(frame.Payload.Bytes()) != "hello" {
		t.Fatalf("frame=%+v payload=%q", frame, frame.Payload.Bytes())
	}
}

type outboundSink struct {
	writes []buffer.ByteBuf
	closes int
}

func (s *outboundSink) Write(msg any) error {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		s.writes = append(s.writes, buf)
	}
	return nil
}
func (s *outboundSink) Flush() error { return nil }
func (s *outboundSink) Close() error {
	s.closes++
	return nil
}
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

type passHandler struct{}

func (passHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	ctx.FireChannelRead(msg)
}

type eventCollector struct {
	events []any
}

func (c *eventCollector) UserEventTriggered(_ *channel.HandlerContext, event any) {
	c.events = append(c.events, event)
}

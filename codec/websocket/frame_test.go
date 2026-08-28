package websocket

import (
	"errors"
	"strings"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/handler/timeout"
)

type frameCollector struct {
	frames []Frame
}

func (c *frameCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	if frame, ok := msg.(Frame); ok {
		c.frames = append(c.frames, frame)
	}
}

type errorCollector struct {
	errs []error
}

func (c *errorCollector) ExceptionCaught(_ *channel.HandlerContext, err error) {
	c.errs = append(c.errs, err)
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

func TestServerFrameDecoderRejectsUnmaskedClientFrame(t *testing.T) {
	decoder, err := NewServerFrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	errs := &errorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("errors", errs)

	ch.Pipeline().FireChannelRead(testBuf([]byte{0x81, 0x02, 'h', 'i'}))
	if len(errs.errs) != 1 || !errors.Is(errs.errs[0], ErrMaskMismatch) {
		t.Fatalf("errs=%v, want ErrMaskMismatch", errs.errs)
	}
}

func TestClientFrameDecoderRejectsMaskedServerFrame(t *testing.T) {
	decoder, err := NewClientFrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	errs := &errorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("errors", errs)

	ch.Pipeline().FireChannelRead(testBuf([]byte{0x81, 0x82, 1, 2, 3, 4, 'h' ^ 1, 'i' ^ 2}))
	if len(errs.errs) != 1 || !errors.Is(errs.errs[0], ErrMaskMismatch) {
		t.Fatalf("errs=%v, want ErrMaskMismatch", errs.errs)
	}
}

func TestFrameDecoderRejectsInvalidCloseStatus(t *testing.T) {
	decoder, err := NewServerFrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	errs := &errorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("errors", errs)

	ch.Pipeline().FireChannelRead(testBuf([]byte{0x88, 0x82, 0, 0, 0, 0, 0x03, 0xed}))
	if len(errs.errs) != 1 || !errors.Is(errs.errs[0], ErrCloseStatusInvalid) {
		t.Fatalf("errs=%v, want ErrCloseStatusInvalid", errs.errs)
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

func TestFrameDecoderRequiresExplicitRSV(t *testing.T) {
	decoder, err := NewFrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	errs := &errorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("errors", errs)

	ch.Pipeline().FireChannelRead(testBuf([]byte{0xc1, 0x00}))
	if len(errs.errs) != 1 || !errors.Is(errs.errs[0], codec.ErrInvalidFrameLength) {
		t.Fatalf("errs=%v, want ErrInvalidFrameLength", errs.errs)
	}
}

func TestFrameDecoderAllowsConfiguredRSV1(t *testing.T) {
	decoder, err := NewFrameDecoderWithConfig(FrameDecoderConfig{MaxFrameLength: 1024, AllowMaskedFrames: true, AllowRSV1: true})
	if err != nil {
		t.Fatal(err)
	}
	collector := &frameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte{0xc1, 0x00}))
	if len(collector.frames) != 1 {
		t.Fatalf("frames=%d, want 1", len(collector.frames))
	}
	frame := collector.frames[0]
	if !frame.Final || frame.Opcode != OpcodeText || !frame.RSV1 || frame.RSV2 || frame.RSV3 {
		t.Fatalf("frame=%+v", frame)
	}
}

func TestFrameEncoderWritesRSV1(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewFrameEncoder())
	defer sink.release()

	if err := ch.Write(Frame{Final: true, Opcode: OpcodeText, RSV1: true}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	if string(sink.writes[0].Bytes()) != string([]byte{0xc1, 0x00}) {
		t.Fatalf("header=%q", sink.writes[0].Bytes())
	}
}

func TestNewCloseFrameRejectsInvalidStatus(t *testing.T) {
	var ctx *channel.HandlerContext
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("capture", handlerAddedFunc(func(c *channel.HandlerContext) error {
		ctx = c
		return nil
	}))
	if _, err := NewCloseFrame(ctx, 1005, ""); !errors.Is(err, ErrCloseStatusInvalid) {
		t.Fatalf("err=%v, want ErrCloseStatusInvalid", err)
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

func TestClientHandshakeWritesRequestAndValidatesResponse(t *testing.T) {
	sink := &outboundSink{}
	events := &eventCollector{}
	handshake, err := NewClientHandshakeHandler(ClientHandshakeConfig{
		URL:            "ws://example.com/chat?room=1",
		Headers:        http1.Headers{"Sec-WebSocket-Protocol": "chat"},
		RemoveHandlers: []string{"handshake"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", http1.NewRequestEncoder())
	_ = ch.Pipeline().AddLast("handshake", handshake)
	_ = ch.Pipeline().AddLast("events", events)
	defer sink.release()

	ch.Pipeline().FireChannelActive()
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	header := string(sink.writes[0].Bytes())
	for _, part := range []string{
		"GET /chat?room=1 HTTP/1.1\r\n",
		"Host: example.com\r\n",
		"Upgrade: websocket\r\n",
		"Connection: Upgrade\r\n",
		"Sec-WebSocket-Key: " + handshake.Key() + "\r\n",
		"Sec-WebSocket-Version: 13\r\n",
		"Sec-WebSocket-Protocol: chat\r\n",
	} {
		if !strings.Contains(header, part) {
			t.Fatalf("request header missing %q in %q", part, header)
		}
	}

	ch.Pipeline().FireChannelRead(http1.Response{
		StatusCode: 101,
		Reason:     "Switching Protocols",
		Headers: http1.Headers{
			"Upgrade":              "websocket",
			"Connection":           "Upgrade",
			"Sec-WebSocket-Accept": AcceptKey(handshake.Key()),
		},
	})
	if _, ok := ch.Pipeline().Context("handshake"); ok {
		t.Fatalf("handshake should be removed after client upgrade")
	}
	if len(events.events) != 1 {
		t.Fatalf("events=%d, want 1", len(events.events))
	}
	complete, ok := events.events[0].(HandshakeComplete)
	if !ok || complete.URI != "/chat?room=1" {
		t.Fatalf("event=%+v", events.events[0])
	}
}

func TestClientHandshakeRejectsInvalidResponse(t *testing.T) {
	sink := &outboundSink{}
	errs := &errorCollector{}
	handshake, err := NewClientHandshakeHandler(ClientHandshakeConfig{URL: "ws://example.com/ws"})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", http1.NewRequestEncoder())
	_ = ch.Pipeline().AddLast("handshake", handshake)
	_ = ch.Pipeline().AddLast("errors", errs)
	defer sink.release()

	ch.Pipeline().FireChannelRead(http1.Response{StatusCode: 200, Reason: "OK", Headers: http1.Headers{}})
	if len(errs.errs) != 1 || !errors.Is(errs.errs[0], ErrInvalidHandshake) {
		t.Fatalf("errs=%v, want ErrInvalidHandshake", errs.errs)
	}
	if sink.closes != 1 {
		t.Fatalf("closes=%d, want 1", sink.closes)
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

func TestControlFrameHandlerTracksOutboundCloseState(t *testing.T) {
	sink := &outboundSink{}
	events := &eventCollector{}
	control := NewControlFrameHandler()
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewFrameEncoder())
	_ = ch.Pipeline().AddLast("control", control)
	_ = ch.Pipeline().AddLast("events", events)
	defer sink.release()

	if err := ch.Write(Frame{Final: true, Opcode: OpcodeClose, Payload: testBuf([]byte{0x03, 0xe8})}); err != nil {
		t.Fatal(err)
	}
	if control.CloseState() != CloseStateCloseSent {
		t.Fatalf("state=%d, want CloseStateCloseSent", control.CloseState())
	}
	ch.Pipeline().FireChannelRead(Frame{Final: true, Opcode: OpcodeClose, Payload: testBuf([]byte{0x03, 0xe8})})
	if control.CloseState() != CloseStateClosed {
		t.Fatalf("state=%d, want CloseStateClosed", control.CloseState())
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want only outbound close frame", len(sink.writes))
	}
	if sink.closes != 1 {
		t.Fatalf("closes=%d, want 1", sink.closes)
	}
	if len(events.events) != 2 {
		t.Fatalf("events=%d, want 2", len(events.events))
	}
}

func TestUTF8ValidatorAcceptsFragmentedText(t *testing.T) {
	collector := &frameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("utf8", NewUTF8Validator())
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(Frame{Final: false, Opcode: OpcodeText, Payload: testBuf([]byte{0xe4, 0xbd})})
	ch.Pipeline().FireChannelRead(Frame{Final: true, Opcode: OpcodeContinuation, Payload: testBuf([]byte{0xa0})})
	if len(collector.frames) != 2 {
		t.Fatalf("frames=%d, want 2", len(collector.frames))
	}
	for _, frame := range collector.frames {
		frame.Release()
	}
}

func TestUTF8ValidatorClosesOnInvalidText(t *testing.T) {
	sink := &outboundSink{}
	errs := &errorCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewFrameEncoder())
	_ = ch.Pipeline().AddLast("utf8", NewUTF8Validator())
	_ = ch.Pipeline().AddLast("errors", errs)
	defer sink.release()

	ch.Pipeline().FireChannelRead(Frame{Final: true, Opcode: OpcodeText, Payload: testBuf([]byte{0xff})})
	if len(errs.errs) != 1 || !errors.Is(errs.errs[0], ErrInvalidUTF8) {
		t.Fatalf("errs=%v, want ErrInvalidUTF8", errs.errs)
	}
	if sink.closes != 1 {
		t.Fatalf("closes=%d, want 1", sink.closes)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want close header and payload", len(sink.writes))
	}
	if status, ok := ParseCloseStatus(Frame{Opcode: OpcodeClose, Payload: sink.writes[1]}); !ok || status.Code != CloseStatusInvalidFrameData {
		t.Fatalf("status=%+v ok=%v", status, ok)
	}
}

func TestIdleHandlerWritesPingOnWriterIdle(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewFrameEncoder())
	_ = ch.Pipeline().AddLast("idle", NewIdleHandler([]byte("hb"), 0, ""))
	defer sink.release()

	ch.Pipeline().FireUserEventTriggered(timeout.IdleStateEvent{State: timeout.WriterIdle, First: true})
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want ping header and payload", len(sink.writes))
	}
	if string(sink.writes[0].Bytes()) != string([]byte{0x89, 0x02}) || string(sink.writes[1].Bytes()) != "hb" {
		t.Fatalf("ping writes=%q,%q", sink.writes[0].Bytes(), sink.writes[1].Bytes())
	}
}

func TestIdleHandlerClosesOnReaderIdle(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewFrameEncoder())
	_ = ch.Pipeline().AddLast("idle", NewIdleHandler(nil, 0, "idle"))
	defer sink.release()

	ch.Pipeline().FireUserEventTriggered(timeout.IdleStateEvent{State: timeout.ReaderIdle, First: true})
	if sink.closes != 1 {
		t.Fatalf("closes=%d, want 1", sink.closes)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want close header and payload", len(sink.writes))
	}
	if status, ok := ParseCloseStatus(Frame{Opcode: OpcodeClose, Payload: sink.writes[1]}); !ok || status.Code != CloseStatusGoingAway || status.Reason != "idle" {
		t.Fatalf("status=%+v ok=%v", status, ok)
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

type handlerAddedFunc func(*channel.HandlerContext) error

func (f handlerAddedFunc) HandlerAdded(ctx *channel.HandlerContext) error {
	return f(ctx)
}

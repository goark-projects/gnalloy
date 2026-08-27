package recipes

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/codec/mqtt"
)

var errRecipeTest = errors.New("recipe test error")

type inboundCapture struct {
	messages []any
	events   []any
}

func (c *inboundCapture) ChannelRead(_ *channel.HandlerContext, msg any) {
	c.messages = append(c.messages, msg)
}

func (c *inboundCapture) UserEventTriggered(_ *channel.HandlerContext, event any) {
	c.events = append(c.events, event)
}

func (c *inboundCapture) release() {
	releaseMessages(c.messages)
	c.messages = nil
}

type testSink struct {
	mu      sync.Mutex
	writes  []any
	flushes int
}

func (s *testSink) Write(msg any) error {
	s.mu.Lock()
	s.writes = append(s.writes, msg)
	s.mu.Unlock()
	return nil
}

func (s *testSink) Flush() error {
	s.mu.Lock()
	s.flushes++
	s.mu.Unlock()
	return nil
}

func (s *testSink) Close() error {
	return nil
}

func (s *testSink) release() {
	s.mu.Lock()
	writes := s.writes
	s.writes = nil
	s.mu.Unlock()
	releaseMessages(writes)
}

type noopHandler struct{}

func TestInitializerRollsBackOnFactoryError(t *testing.T) {
	ch, _ := newRecipeChannel()
	initializer := Initializer(
		Use("first", noopHandler{}),
		UseFactory("bad", func() (channel.Handler, error) {
			return nil, errRecipeTest
		}),
	)

	err := initializer(ch)
	if !errors.Is(err, errRecipeTest) {
		t.Fatalf("err=%v, want %v", err, errRecipeTest)
	}
	if _, ok := ch.Pipeline().Context("first"); ok {
		t.Fatal("first handler should be removed after factory failure")
	}
}

func TestLengthFieldFramesRoundTrip(t *testing.T) {
	capture := &inboundCapture{}
	ch, sink := newRecipeChannel()
	initializer := LengthFieldFrames(LengthFieldConfig{MaxFrameLength: 64}, Use("capture", capture))
	if err := initializer(ch); err != nil {
		t.Fatal(err)
	}
	defer capture.release()
	defer sink.release()

	wantNames := []string{HandlerNameLengthFieldDecoder, HandlerNameLengthFieldPrepender, "capture"}
	if got := ch.Pipeline().Names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("names=%v, want %v", got, wantNames)
	}

	ch.Pipeline().FireChannelRead(testBuffer([]byte{0, 0, 0, 4, 'p', 'i', 'n', 'g'}))
	if len(capture.messages) != 1 {
		t.Fatalf("messages=%d, want 1", len(capture.messages))
	}
	frame := capture.messages[0].(buffer.ByteBuf)
	if !bytes.Equal(frame.Bytes(), []byte("ping")) {
		t.Fatalf("frame=%q, want ping", frame.Bytes())
	}

	out := testBuffer([]byte("pong"))
	if err := ch.WriteAndFlush(out); err != nil {
		t.Fatal(err)
	}
	raw := concatWrites(t, sink.writes)
	if !bytes.Equal(raw, []byte{0, 0, 0, 4, 'p', 'o', 'n', 'g'}) {
		t.Fatalf("outbound=%v, want length-prefixed pong", raw)
	}
	if sink.flushes != 1 {
		t.Fatalf("flushes=%d, want 1", sink.flushes)
	}
}

func TestHTTP1ServerRecipe(t *testing.T) {
	capture := &inboundCapture{}
	ch, sink := newRecipeChannel()
	if err := HTTP1Server(HTTP1Config{}, Use("capture", capture))(ch); err != nil {
		t.Fatal(err)
	}
	defer capture.release()
	defer sink.release()

	ch.Pipeline().FireChannelRead(testBuffer([]byte("GET /ok HTTP/1.1\r\nHost: example\r\n\r\n")))
	if len(capture.messages) != 1 {
		t.Fatalf("messages=%d, want 1", len(capture.messages))
	}
	req := capture.messages[0].(http1.Request)
	if req.Method != "GET" || req.URI != "/ok" || req.Headers.Get("Host") != "example" {
		t.Fatalf("request=%+v", req)
	}

	if err := ch.WriteAndFlush(http1.Response{StatusCode: 200, Headers: http1.Headers{"Server": "gnalloy"}}); err != nil {
		t.Fatal(err)
	}
	if raw := string(concatWrites(t, sink.writes)); !strings.Contains(raw, "HTTP/1.1 200 OK") || !strings.Contains(raw, "Server: gnalloy") {
		t.Fatalf("response=%q", raw)
	}
}

func TestWebSocketServerRecipeUpgrade(t *testing.T) {
	capture := &inboundCapture{}
	ch, sink := newRecipeChannel()
	if err := WebSocketServer(WebSocketServerConfig{Path: "/ws", ValidateUTF8: true, AggregateFragments: true}, Use("capture", capture))(ch); err != nil {
		t.Fatal(err)
	}
	defer capture.release()
	defer sink.release()

	request := "GET /ws HTTP/1.1\r\n" +
		"Host: example\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	ch.Pipeline().FireChannelRead(testBuffer([]byte(request)))

	raw := string(concatWrites(t, sink.writes))
	if !strings.Contains(raw, "HTTP/1.1 101 Switching Protocols") || !strings.Contains(raw, "Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=") {
		t.Fatalf("upgrade response=%q", raw)
	}
	for _, removed := range []string{HandlerNameHTTP1RequestDecoder, HandlerNameHTTP1ResponseEncoder, HandlerNameHTTP1Continue, HandlerNameWebSocketHandshake} {
		if _, ok := ch.Pipeline().Context(removed); ok {
			t.Fatalf("handler %s should be removed after upgrade", removed)
		}
	}
	if len(capture.events) != 1 {
		t.Fatalf("events=%d, want handshake complete", len(capture.events))
	}
}

func TestMQTTFramesRecipe(t *testing.T) {
	capture := &inboundCapture{}
	ch, sink := newRecipeChannel()
	if err := MQTTFrames(MQTTConfig{Typed: true}, Use("capture", capture))(ch); err != nil {
		t.Fatal(err)
	}
	defer capture.release()
	defer sink.release()

	ch.Pipeline().FireChannelRead(testBuffer([]byte{0xc0, 0}))
	if len(capture.messages) != 1 {
		t.Fatalf("messages=%d, want 1", len(capture.messages))
	}
	frame := capture.messages[0].(mqtt.Frame)
	if frame.PacketType() != mqtt.PacketPingReq {
		t.Fatalf("packet type=%d, want ping req", frame.PacketType())
	}

	if err := ch.WriteAndFlush(mqtt.PingResp()); err != nil {
		t.Fatal(err)
	}
	if raw := concatWrites(t, sink.writes); !bytes.Equal(raw, []byte{0xd0, 0}) {
		t.Fatalf("outbound=%v, want ping resp", raw)
	}
}

func TestHTTP2ConnectionRecipeInstallsPipeline(t *testing.T) {
	ch, _ := newRecipeChannel()
	if err := HTTP2Connection(HTTP2Config{}, Use("capture", &inboundCapture{}))(ch); err != nil {
		t.Fatal(err)
	}
	want := []string{
		HandlerNameHTTP2FrameDecoder,
		HandlerNameHTTP2TypedDecoder,
		HandlerNameHTTP2FrameEncoder,
		HandlerNameHTTP2TypedEncoder,
		HandlerNameHTTP2Multiplexer,
		"capture",
	}
	if got := ch.Pipeline().Names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("names=%v, want %v", got, want)
	}
}

func newRecipeChannel() (*channel.LocalChannel, *testSink) {
	sink := &testSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	return ch, sink
}

func testBuffer(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

func concatWrites(t *testing.T, writes []any) []byte {
	t.Helper()
	var out []byte
	for _, msg := range writes {
		buf, ok := msg.(buffer.ByteBuf)
		if !ok {
			t.Fatalf("message=%T, want ByteBuf", msg)
		}
		out = append(out, buf.Bytes()...)
	}
	return out
}

func releaseMessages(messages []any) {
	for _, msg := range messages {
		switch value := msg.(type) {
		case buffer.ByteBuf:
			value.Release()
		case interface{ Release() }:
			value.Release()
		}
	}
}

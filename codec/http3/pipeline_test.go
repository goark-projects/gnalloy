package http3

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestRequestStreamPipelineRoundTripsHeadersBlock(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ApplyRequestStreamPipeline(ch.Pipeline(), PipelineConfig{}); err != nil {
		t.Fatal(err)
	}
	inbound := &pipelineInboundCapture{}
	if err := ch.Pipeline().AddLast("capture", inbound); err != nil {
		t.Fatal(err)
	}
	defer sink.release()
	defer inbound.release()

	wantNames := []string{
		HandlerNameHTTP3FrameDecoder,
		HandlerNameHTTP3HeaderDecoder,
		HandlerNameHTTP3FrameEncoder,
		HandlerNameHTTP3HeaderEncoder,
		"capture",
	}
	if got := ch.Pipeline().Names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("pipeline names=%v, want %v", got, wantNames)
	}

	if err := ch.Write(HeadersBlock{Fields: []HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":path", Value: "/items"},
		{Name: "x-trace", Value: "abc"},
	}}); err != nil {
		t.Fatal(err)
	}

	raw := collectByteBufs(t, sink.writes)
	if _, err := raw.WriteBytes(rawBytes(t, sink.writes)); err != nil {
		t.Fatal(err)
	}
	if raw.ReadableBytes() == 0 {
		t.Fatal("missing encoded HTTP/3 bytes")
	}

	ch.Pipeline().FireChannelRead(raw)
	if len(inbound.messages) != 1 {
		t.Fatalf("inbound messages=%d, want 1", len(inbound.messages))
	}
	headers, ok := inbound.messages[0].(HeadersBlock)
	if !ok {
		t.Fatalf("message=%T, want HeadersBlock", inbound.messages[0])
	}
	if len(headers.Fields) != 3 || headers.Fields[2].Name != "x-trace" || headers.Fields[2].Value != "abc" {
		t.Fatalf("headers=%+v", headers)
	}
}

func TestRemoteControlStreamPipelineValidatesStreamFrames(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ApplyRemoteControlStreamPipeline(ch.Pipeline(), PipelineConfig{}); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("exception", http3ExceptionCapture{}); err != nil {
		t.Fatal(err)
	}
	inbound := &pipelineInboundCapture{}
	if err := ch.Pipeline().AddLast("capture", inbound); err != nil {
		t.Fatal(err)
	}
	defer sink.release()
	defer inbound.release()

	ch.Pipeline().FireChannelRead(testBuf([]byte{byte(StreamTypeControl), byte(FrameSettings), 2, 1, 10}))
	if len(inbound.messages) != 2 {
		t.Fatalf("inbound messages=%d, want stream type and settings", len(inbound.messages))
	}
	if st, ok := inbound.messages[0].(StreamTypeFrame); !ok || st.Type != StreamTypeControl {
		t.Fatalf("stream type=%v, want control", inbound.messages[0])
	}
	if settings, ok := inbound.messages[1].(SettingsFrame); !ok || len(settings.Settings) != 1 || settings.Settings[0].Value != 10 {
		t.Fatalf("settings=%+v", inbound.messages[1])
	}

	ch.Pipeline().FireChannelRead(testBuf([]byte{byte(FrameData), 0}))
	got := inbound.messages[len(inbound.messages)-1]
	if err, ok := got.(error); !ok || !errors.Is(err, ErrUnsupportedFrame) {
		t.Fatalf("error=%v, want ErrUnsupportedFrame", got)
	}
}

func TestLocalControlStreamPipelineWritesSettingsOnActive(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ApplyLocalControlStreamPipeline(ch.Pipeline(), PipelineConfig{
		Settings: []Setting{{ID: 1, Value: 10}, {ID: 7, Value: 11}},
	}); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	ch.Pipeline().FireChannelActive()
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want stream type prefix and settings frame", len(sink.writes))
	}
	if got := sink.writes[0].(buffer.ByteBuf).Bytes(); !bytes.Equal(got, []byte{byte(StreamTypeControl)}) {
		t.Fatalf("stream type bytes=%v", got)
	}
	if got := sink.writes[1].(buffer.ByteBuf).Bytes(); !bytes.Equal(got, []byte{byte(FrameSettings), 4, 1, 10, 7, 11}) {
		t.Fatalf("settings bytes=%v", got)
	}
	if sink.flushes != 1 {
		t.Fatalf("flushes=%d, want 1", sink.flushes)
	}
}

func TestQPACKStreamPipelinesWriteExpectedStreamTypes(t *testing.T) {
	tests := []struct {
		name      string
		apply     func(*channel.Pipeline) error
		streamTyp StreamType
	}{
		{name: "encoder", apply: ApplyQPACKEncoderStreamPipeline, streamTyp: StreamTypeQPACKEncoder},
		{name: "decoder", apply: ApplyQPACKDecoderStreamPipeline, streamTyp: StreamTypeQPACKDecoder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := &captureSink{}
			ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
			if err := tt.apply(ch.Pipeline()); err != nil {
				t.Fatal(err)
			}
			defer sink.release()

			if err := ch.Write(testBuf([]byte{0x40})); err != nil {
				t.Fatal(err)
			}
			if len(sink.writes) != 2 {
				t.Fatalf("writes=%d, want stream type prefix and payload", len(sink.writes))
			}
			if got := sink.writes[0].(buffer.ByteBuf).Bytes(); !bytes.Equal(got, []byte{byte(tt.streamTyp)}) {
				t.Fatalf("stream type bytes=%v, want %d", got, tt.streamTyp)
			}
		})
	}
}

func TestHTTP3PipelineRejectsInvalidConfig(t *testing.T) {
	if err := ApplyRequestStreamPipeline(nil, PipelineConfig{}); !errors.Is(err, ErrInvalidPipeline) {
		t.Fatalf("err=%v, want ErrInvalidPipeline", err)
	}

	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ApplyRequestStreamPipeline(ch.Pipeline(), PipelineConfig{}); err != nil {
		t.Fatal(err)
	}
	if err := ApplyRequestStreamPipeline(ch.Pipeline(), PipelineConfig{}); !errors.Is(err, channel.ErrDuplicateHandler) {
		t.Fatalf("err=%v, want duplicate handler", err)
	}
}

func TestRequestStreamInitializerMatchesBootstrapCallback(t *testing.T) {
	var initializer bootstrap.ChildInitializer = RequestStreamInitializer(PipelineConfig{})

	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := initializer(ch); err != nil {
		t.Fatal(err)
	}
	if _, ok := ch.Pipeline().Context(HandlerNameHTTP3FrameDecoder); !ok {
		t.Fatal("missing frame decoder")
	}
}

type pipelineInboundCapture struct {
	messages []any
}

func (c *pipelineInboundCapture) ChannelRead(_ *channel.HandlerContext, msg any) {
	c.messages = append(c.messages, msg)
}

func (c *pipelineInboundCapture) release() {
	for _, msg := range c.messages {
		releaseMessage(msg)
	}
}

func rawBytes(t *testing.T, messages []any) []byte {
	t.Helper()
	var out []byte
	for _, msg := range messages {
		buf, ok := msg.(buffer.ByteBuf)
		if !ok {
			t.Fatalf("message=%T, want ByteBuf", msg)
		}
		out = append(out, buf.Bytes()...)
	}
	return out
}

func collectByteBufs(t *testing.T, messages []any) buffer.ByteBuf {
	t.Helper()
	total := 0
	for _, msg := range messages {
		buf, ok := msg.(buffer.ByteBuf)
		if !ok {
			t.Fatalf("message=%T, want ByteBuf", msg)
		}
		total += buf.ReadableBytes()
	}
	return buffer.NewHeapBuffer(total)
}

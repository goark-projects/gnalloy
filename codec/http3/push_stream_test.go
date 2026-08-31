package http3

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestRemotePushStreamPipelineReadsPushIDAndFrames(t *testing.T) {
	state, err := NewStateManager(StateManagerConfig{InitialMaxPushID: 7})
	if err != nil {
		t.Fatal(err)
	}
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ApplyRemotePushStreamPipeline(ch.Pipeline(), PipelineConfig{State: state}); err != nil {
		t.Fatal(err)
	}
	capture := &pipelineInboundCapture{}
	if err := ch.Pipeline().AddLast("capture", capture); err != nil {
		t.Fatal(err)
	}
	defer sink.release()
	defer capture.release()

	wantNames := []string{
		HandlerNameHTTP3StreamTypeDecoder,
		HandlerNameHTTP3StreamTypeGuard,
		HandlerNameHTTP3PushIDDecoder,
		HandlerNameHTTP3StateManager,
		HandlerNameHTTP3FrameDecoder,
		HandlerNameHTTP3HeaderDecoder,
		HandlerNameHTTP3FrameEncoder,
		HandlerNameHTTP3HeaderEncoder,
		"capture",
	}
	if got := ch.Pipeline().Names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("pipeline names=%v, want %v", got, wantNames)
	}

	ch.Pipeline().FireChannelRead(testBuf([]byte{byte(StreamTypePush), 3, byte(FrameHeaders), 0}))
	if len(capture.messages) != 3 {
		t.Fatalf("messages=%d, want stream type, push id and headers", len(capture.messages))
	}
	if id, ok := capture.messages[1].(PushIDFrame); !ok || id.PushID != 3 {
		t.Fatalf("push id=%+v", capture.messages[1])
	}
	if headers, ok := capture.messages[2].(HeadersBlock); !ok || len(headers.Fields) != 0 {
		t.Fatalf("headers=%+v", capture.messages[2])
	}
	if got := state.PushState(3); got != PushStatePromised {
		t.Fatalf("push state=%d, want promised", got)
	}
}

func TestRemotePushStreamPipelineRejectsServerSideReceive(t *testing.T) {
	state, err := NewStateManager(StateManagerConfig{Server: true, InitialMaxPushID: 7})
	if err != nil {
		t.Fatal(err)
	}
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ApplyRemotePushStreamPipeline(ch.Pipeline(), PipelineConfig{State: state}); err != nil {
		t.Fatal(err)
	}
	exceptions := &pushExceptionCapture{}
	if err := ch.Pipeline().AddLast("exception", exceptions); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	ch.Pipeline().FireChannelRead(testBuf([]byte{byte(StreamTypePush), 1}))
	if len(exceptions.errs) != 1 || !errors.Is(exceptions.errs[0], ErrInvalidFrame) {
		t.Fatalf("errors=%v, want ErrInvalidFrame", exceptions.errs)
	}
}

func TestLocalPushStreamPipelineWritesTypeAndPushID(t *testing.T) {
	state, err := NewStateManager(StateManagerConfig{Server: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := state.readMaxPushID(MaxPushIDFrame{PushID: 4}); err != nil {
		t.Fatal(err)
	}
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ApplyLocalPushStreamPipeline(ch.Pipeline(), PipelineConfig{State: state}, 2); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	ch.Pipeline().FireChannelActive()
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want stream type and push id", len(sink.writes))
	}
	if got := rawBytes(t, sink.writes); !bytes.Equal(got, []byte{byte(StreamTypePush), 2}) {
		t.Fatalf("raw=%v, want push stream prefix", got)
	}
	if sink.flushes != 1 {
		t.Fatalf("flushes=%d, want 1", sink.flushes)
	}
}

type pushExceptionCapture struct {
	errs []error
}

func (c *pushExceptionCapture) ExceptionCaught(_ *channel.HandlerContext, err error) {
	c.errs = append(c.errs, err)
}

package http2

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/channel/embedded"
)

func TestStreamChildHandlerRoutesInboundFramesToChild(t *testing.T) {
	recorder := &childRecorder{}
	children, err := NewStreamChildHandler(StreamChildConfig{
		Initializer: func(ch *StreamChannel) error {
			if ch.StreamID() != 1 {
				t.Fatalf("stream id=%d, want 1", ch.StreamID())
			}
			return ch.Pipeline().AddLast("recorder", recorder)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mux, err := NewStreamMultiplexer(MultiplexerConfig{Server: true})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := embedded.New(mux, children)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.FinishAndReleaseAll()

	if _, err := parent.WriteInbound(HeadersBlock{StreamID: 1, Fields: []HeaderField{{Name: ":method", Value: "GET"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.WriteInbound(DataFrame{StreamID: 1, Data: testHTTP2Buf(t, "body")}); err != nil {
		t.Fatal(err)
	}

	if recorder.active != 1 {
		t.Fatalf("active=%d, want 1", recorder.active)
	}
	if len(recorder.reads) != 2 {
		t.Fatalf("reads=%d, want 2", len(recorder.reads))
	}
	if _, ok := recorder.reads[0].(HeadersBlock); !ok {
		t.Fatalf("first read=%T, want HeadersBlock", recorder.reads[0])
	}
	if frame, ok := recorder.reads[1].(DataFrame); !ok || string(frame.Data.Bytes()) != "body" {
		t.Fatalf("second read=%T %+v, want DataFrame body", recorder.reads[1], recorder.reads[1])
	}
	recorder.release()
}

func TestStreamChildHandlerBindsOutboundStreamID(t *testing.T) {
	children, err := NewStreamChildHandler(StreamChildConfig{
		Initializer: func(*StreamChannel) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	mux, err := NewStreamMultiplexer(MultiplexerConfig{Server: true})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := embedded.New(mux, children)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.FinishAndReleaseAll()

	if _, err := parent.WriteInbound(HeadersBlock{StreamID: 1}); err != nil {
		t.Fatal(err)
	}
	child, ok := children.Child(1)
	if !ok {
		t.Fatal("missing child channel")
	}
	if err := child.WriteAndFlush(testHTTP2Buf(t, "reply")); err != nil {
		t.Fatal(err)
	}

	msg, ok := parent.ReadOutbound()
	if !ok {
		t.Fatal("missing outbound frame")
	}
	defer releaseChildMessage(msg)
	frame, ok := msg.(DataFrame)
	if !ok {
		t.Fatalf("msg=%T, want DataFrame", msg)
	}
	if frame.StreamID != 1 || string(frame.Data.Bytes()) != "reply" {
		t.Fatalf("frame=%+v, want stream 1 reply", frame)
	}
}

func TestStreamChildHandlerClosesChildOnRST(t *testing.T) {
	recorder := &childRecorder{}
	children, err := NewStreamChildHandler(StreamChildConfig{
		Initializer: func(ch *StreamChannel) error {
			return ch.Pipeline().AddLast("recorder", recorder)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mux, err := NewStreamMultiplexer(MultiplexerConfig{Server: true})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := embedded.New(mux, children)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.FinishAndReleaseAll()

	if _, err := parent.WriteInbound(HeadersBlock{StreamID: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.WriteInbound(RSTStreamFrame{StreamID: 1, ErrorCode: ErrorCodeCancel}); err != nil {
		t.Fatal(err)
	}

	if recorder.inactive != 1 {
		t.Fatalf("inactive=%d, want 1", recorder.inactive)
	}
	if children.ActiveChildren() != 0 {
		t.Fatalf("children=%d, want 0", children.ActiveChildren())
	}
	recorder.release()
}

func TestStreamChildRejectsMismatchedStreamID(t *testing.T) {
	children, err := NewStreamChildHandler(StreamChildConfig{
		Initializer: func(*StreamChannel) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	mux, err := NewStreamMultiplexer(MultiplexerConfig{Server: true})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := embedded.New(mux, children)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.FinishAndReleaseAll()

	if _, err := parent.WriteInbound(HeadersBlock{StreamID: 1}); err != nil {
		t.Fatal(err)
	}
	child, ok := children.Child(1)
	if !ok {
		t.Fatal("missing child channel")
	}
	err = child.Write(DataFrame{StreamID: 3, Data: testHTTP2Buf(t, "bad")})
	if !errors.Is(err, ErrInvalidStreamID) {
		t.Fatalf("err=%v, want ErrInvalidStreamID", err)
	}
}

type childRecorder struct {
	active   int
	inactive int
	reads    []any
}

func (r *childRecorder) ChannelActive(*channel.HandlerContext) {
	r.active++
}

func (r *childRecorder) ChannelInactive(*channel.HandlerContext) {
	r.inactive++
}

func (r *childRecorder) ChannelRead(_ *channel.HandlerContext, msg any) {
	r.reads = append(r.reads, msg)
}

func (r *childRecorder) release() {
	for _, msg := range r.reads {
		releaseChildMessage(msg)
	}
	r.reads = r.reads[:0]
}

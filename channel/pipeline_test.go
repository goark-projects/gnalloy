package channel

import (
	"testing"

	"github.com/goark-projects/gnalloy/buffer"
)

type captureInbound struct {
	msgs []any
}

func (h *captureInbound) ChannelRead(_ *HandlerContext, msg any) {
	h.msgs = append(h.msgs, msg)
}

type forwardingInbound struct{}

func (forwardingInbound) ChannelRead(ctx *HandlerContext, msg any) {
	ctx.FireChannelRead(msg)
}

type captureSink struct {
	writes  []any
	flushed bool
	closed  bool
}

func (s *captureSink) Write(msg any) error {
	s.writes = append(s.writes, msg)
	return nil
}

func (s *captureSink) Flush() error {
	s.flushed = true
	return nil
}

func (s *captureSink) Close() error {
	s.closed = true
	return nil
}

func TestPipelineInboundPropagation(t *testing.T) {
	ch := NewLocalChannel(1, buffer.NewHeapAllocator(), &captureSink{})
	capture := &captureInbound{}
	if err := ch.Pipeline().AddLast("forward", forwardingInbound{}); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("capture", capture); err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireChannelRead("hello")
	if len(capture.msgs) != 1 || capture.msgs[0] != "hello" {
		t.Fatalf("msgs=%v", capture.msgs)
	}
}

func TestPipelineOutboundSink(t *testing.T) {
	sink := &captureSink{}
	ch := NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().Write("payload"); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().Flush(); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().Close(); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 || sink.writes[0] != "payload" || !sink.flushed || !sink.closed {
		t.Fatalf("sink=%+v", sink)
	}
}

func TestPipelineTailReleasesByteBuf(t *testing.T) {
	ch := NewLocalChannel(1, buffer.NewHeapAllocator(), &captureSink{})
	buf := buffer.NewHeapBuffer(8)
	_, _ = buf.WriteBytes([]byte("abc"))
	ch.Pipeline().FireChannelRead(buf)
	if buf.RefCnt() != 0 {
		t.Fatalf("ref=%d, want 0", buf.RefCnt())
	}
}

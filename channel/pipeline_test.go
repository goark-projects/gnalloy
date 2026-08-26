package channel

import (
	"testing"

	"goark.dev/gnalloy/buffer"
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

func TestPipelineAddBeforeAfterAndReplaceKeepOrder(t *testing.T) {
	ch := NewLocalChannel(1, buffer.NewHeapAllocator(), &captureSink{})
	pipeline := ch.Pipeline()
	if err := pipeline.AddLast("first", forwardingInbound{}); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.AddAfter("first", "third", forwardingInbound{}); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.AddBefore("third", "second", forwardingInbound{}); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Replace("third", "last", forwardingInbound{}); err != nil {
		t.Fatal(err)
	}
	got := pipeline.Names()
	want := []string{"first", "second", "last"}
	if len(got) != len(want) {
		t.Fatalf("names=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("names=%v, want %v", got, want)
		}
	}
	first, ok := pipeline.FirstContext()
	if !ok || first.Name() != "first" {
		t.Fatalf("first=%v ok=%v", first, ok)
	}
	last, ok := pipeline.LastContext()
	if !ok || last.Name() != "last" {
		t.Fatalf("last=%v ok=%v", last, ok)
	}
	if _, ok := pipeline.Context("third"); ok {
		t.Fatal("old handler name still exists after replace")
	}
}

func BenchmarkPipelineInboundNoop(b *testing.B) {
	ch := NewLocalChannel(1, buffer.NewHeapAllocator(), &captureSink{})
	if err := ch.Pipeline().AddLast("forward", forwardingInbound{}); err != nil {
		b.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("capture", &captureInbound{}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ch.Pipeline().FireChannelRead("msg")
	}
}

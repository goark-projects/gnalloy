package testutil

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

type writeHandler interface {
	Write(ctx *channel.HandlerContext, msg any) error
}

type readHandler interface {
	ChannelRead(ctx *channel.HandlerContext, msg any)
}

type Sink struct {
	Writes []any
}

func (s *Sink) Write(msg any) error {
	s.Writes = append(s.Writes, msg)
	return nil
}

func (s *Sink) Flush() error { return nil }
func (s *Sink) Close() error { return nil }

type Collector struct {
	Reads []any
	Err   error
}

func (c *Collector) ChannelRead(_ *channel.HandlerContext, msg any) {
	c.Reads = append(c.Reads, msg)
}

func (c *Collector) ExceptionCaught(_ *channel.HandlerContext, err error) {
	c.Err = err
}

func EncodeWithHandler(t testing.TB, encoder writeHandler, payload []byte) buffer.ByteBuf {
	t.Helper()
	sink := &Sink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("encoder", encoder); err != nil {
		t.Fatal(err)
	}
	in := Buffer(payload)
	if err := ch.Write(in); err != nil {
		t.Fatal(err)
	}
	if in.RefCnt() != 0 {
		t.Fatalf("input ref=%d, want 0", in.RefCnt())
	}
	if len(sink.Writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.Writes))
	}
	return sink.Writes[0].(buffer.ByteBuf)
}

func DecodeWithHandler(t *testing.T, decoder readHandler, compressed buffer.ByteBuf) buffer.ByteBuf {
	t.Helper()
	collector := DecodeWithCollector(t, decoder, compressed)
	if collector.Err != nil {
		t.Fatal(collector.Err)
	}
	if len(collector.Reads) != 1 {
		t.Fatalf("reads=%d, want 1", len(collector.Reads))
	}
	return collector.Reads[0].(buffer.ByteBuf)
}

func DecodeWithCollector(t testing.TB, decoder readHandler, compressed buffer.ByteBuf) *Collector {
	t.Helper()
	collector := &Collector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("decoder", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireChannelRead(compressed)
	return collector
}

func Buffer(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

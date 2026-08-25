package redis

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

type frameCollector struct {
	frames []buffer.ByteBuf
}

func (c *frameCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		c.frames = append(c.frames, buf)
	}
}

type valueCollector struct {
	values []Value
}

func (c *valueCollector) ChannelRead(_ *channel.HandlerContext, msg any) {
	if value, ok := msg.(Value); ok {
		c.values = append(c.values, value)
	}
}

func TestFrameDecoderBulkString(t *testing.T) {
	decoder, err := NewFrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &frameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte("$5\r\nhe")))
	ch.Pipeline().FireChannelRead(testBuf([]byte("llo\r\n")))
	if len(collector.frames) != 1 || string(collector.frames[0].Bytes()) != "$5\r\nhello\r\n" {
		t.Fatalf("frames=%v", collector.frames)
	}
	collector.frames[0].Release()
}

func TestCommandEncoder(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewCommandEncoder())
	defer sink.release()

	if err := ch.Write([][]byte{[]byte("PING"), []byte("x")}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 || string(sink.writes[0].Bytes()) != "*2\r\n$4\r\nPING\r\n$1\r\nx\r\n" {
		t.Fatalf("writes=%q", sink.writes[0].Bytes())
	}
}

func TestValueDecoderArrayCommand(t *testing.T) {
	frameDecoder, err := NewFrameDecoder(1024)
	if err != nil {
		t.Fatal(err)
	}
	collector := &valueCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("frame", frameDecoder)
	_ = ch.Pipeline().AddLast("value", NewValueDecoder())
	_ = ch.Pipeline().AddLast("collector", collector)

	ch.Pipeline().FireChannelRead(testBuf([]byte("*2\r\n$4\r\nPI")))
	ch.Pipeline().FireChannelRead(testBuf([]byte("NG\r\n$1\r\nx\r\n")))
	if len(collector.values) != 1 {
		t.Fatalf("values=%d, want 1", len(collector.values))
	}
	defer collector.values[0].Release()
	array, ok := collector.values[0].(Array)
	if !ok || len(array.Values) != 2 {
		t.Fatalf("value=%#v", collector.values[0])
	}
	first := array.Values[0].(BulkString)
	second := array.Values[1].(BulkString)
	if string(first.Data.Bytes()) != "PING" || string(second.Data.Bytes()) != "x" {
		t.Fatalf("array=%q,%q", first.Data.Bytes(), second.Data.Bytes())
	}
}

func TestValueEncoder(t *testing.T) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewValueEncoder())
	defer sink.release()

	if err := ch.Write(SimpleString{Value: "PONG"}); err != nil {
		t.Fatal(err)
	}
	if err := ch.Write(Integer{Value: 7}); err != nil {
		t.Fatal(err)
	}
	if err := ch.Write(Array{Values: []Value{BulkString{Data: testBuf([]byte("PING"))}, BulkString{Data: testBuf([]byte("x"))}}}); err != nil {
		t.Fatal(err)
	}
	got := ""
	for _, buf := range sink.writes {
		got += string(buf.Bytes())
	}
	want := "+PONG\r\n:7\r\n*2\r\n$4\r\nPING\r\n$1\r\nx\r\n"
	if got != want {
		t.Fatalf("encoded=%q want=%q", got, want)
	}
}

type outboundSink struct{ writes []buffer.ByteBuf }

func (s *outboundSink) Write(msg any) error {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		s.writes = append(s.writes, buf)
	}
	return nil
}
func (s *outboundSink) Flush() error { return nil }
func (s *outboundSink) Close() error { return nil }
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

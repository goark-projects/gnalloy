package stomp

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestDecoderParsesFrameWithContentLengthAndNULInBody(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	decoder, err := NewDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("stomp", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	in := testBuf("SEND\ndestination:/q\ncontent-length:5\n\nab\x00cd\x00\n")
	ch.Pipeline().FireChannelRead(in)
	if len(collector.msgs) != 1 {
		t.Fatalf("msgs=%d, want 1", len(collector.msgs))
	}
	frame := collector.msgs[0].(Frame)
	defer frame.Release()
	if frame.Command != CommandSend || frame.Headers.Get("destination") != "/q" {
		t.Fatalf("frame=%+v", frame)
	}
	if string(frame.Body.Bytes()) != "ab\x00cd" {
		t.Fatalf("body=%q", frame.Body.Bytes())
	}
	if in.RefCnt() != 1 {
		t.Fatalf("input ref=%d, want 1 while body slice is alive", in.RefCnt())
	}
}

func TestDecoderParsesHeartbeatAndEscapedHeaders(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	decoder, err := NewDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("stomp", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(testBuf("\nMESSAGE\nh:a\\cb\\n\n\nbody\x00"))
	if len(collector.msgs) != 2 {
		t.Fatalf("msgs=%d, want 2", len(collector.msgs))
	}
	heartbeat := collector.msgs[0].(Frame)
	if !heartbeat.Heartbeat {
		t.Fatalf("heartbeat=%+v", heartbeat)
	}
	frame := collector.msgs[1].(Frame)
	defer frame.Release()
	if got := frame.Headers.Get("h"); got != "a:b\n" {
		t.Fatalf("header=%q", got)
	}
	if string(frame.Body.Bytes()) != "body" {
		t.Fatalf("body=%q", frame.Body.Bytes())
	}
}

func TestEncoderWritesFrameAndKeepsBodyOwnershipWithSink(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("stomp", NewEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	body := testBuf("hello")
	headers := Headers{{Name: "destination", Value: "/q"}, {Name: "h", Value: "a:b\n"}}
	if err := ch.Write(Frame{Command: CommandSend, Headers: headers, Body: body}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 3 {
		t.Fatalf("writes=%d, want 3", len(sink.writes))
	}
	if got := string(sink.writes[0].(buffer.ByteBuf).Bytes()); got != "SEND\ndestination:/q\nh:a\\cb\\n\ncontent-length:5\n\n" {
		t.Fatalf("header=%q", got)
	}
	if sink.writes[1] != body {
		t.Fatal("body should be passed through without copy")
	}
	if got := string(sink.writes[2].(buffer.ByteBuf).Bytes()); got != "\x00" {
		t.Fatalf("tail=%q", got)
	}
}

func TestEncoderWritesHeartbeat(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("stomp", NewEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	if err := ch.Write(Heartbeat()); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	if got := string(sink.writes[0].(buffer.ByteBuf).Bytes()); got != "\n" {
		t.Fatalf("heartbeat=%q", got)
	}
}

func BenchmarkDecoderContentLengthFrame(b *testing.B) {
	decoder, err := NewDecoder(1024, 1024)
	if err != nil {
		b.Fatal(err)
	}
	payload := []byte("SEND\ndestination:/q\ncontent-length:5\n\nhello\x00")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		in := testBuf(string(payload))
		comp := singleComposite(in)
		out, err := decoder.Decode(nil, comp)
		if err != nil {
			b.Fatal(err)
		}
		frame := out.(Frame)
		frame.Release()
		comp.Release()
	}
}

func BenchmarkEncoderFrame(b *testing.B) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("stomp", NewEncoder()); err != nil {
		b.Fatal(err)
	}
	headers := Headers{{Name: "destination", Value: "/q"}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := ch.Write(Frame{Command: CommandSend, Headers: headers, Body: testBuf("hello")}); err != nil {
			b.Fatal(err)
		}
		sink.release()
		sink.writes = nil
	}
}

type captureInbound struct {
	msgs []any
}

func (h *captureInbound) ChannelRead(_ *channel.HandlerContext, msg any) {
	h.msgs = append(h.msgs, msg)
}

type captureSink struct {
	writes []any
}

func (s *captureSink) Write(msg any) error {
	s.writes = append(s.writes, msg)
	return nil
}

func (s *captureSink) Flush() error {
	return nil
}

func (s *captureSink) Close() error {
	return nil
}

func (s *captureSink) release() {
	for _, msg := range s.writes {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
}

func testBuf(s string) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(s))
	_, _ = buf.WriteBytes([]byte(s))
	return buf
}

func singleComposite(buf buffer.ByteBuf) *buffer.CompositeByteBuf {
	comp := buffer.NewCompositeByteBuf()
	comp.Append(buf)
	return comp
}

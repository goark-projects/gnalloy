package smtp

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestResponseDecoderAggregatesMultilineResponse(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	decoder, err := NewResponseDecoder(256)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("smtp", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(testBuf("250-mail.example\r\n250-PIPELINING\r\n250 SIZE 1024\r\n"))
	if len(collector.msgs) != 1 {
		t.Fatalf("msgs=%d, want 1", len(collector.msgs))
	}
	resp := collector.msgs[0].(Response)
	if resp.Code != 250 || len(resp.Details) != 3 || resp.Details[2] != "SIZE 1024" {
		t.Fatalf("resp=%+v", resp)
	}
}

func TestRequestEncoderWritesCommandLine(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("smtp", NewRequestEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	if err := ch.Write(NewRequest(CommandMAIL, "FROM:<a@example.com>", "SIZE=5")); err != nil {
		t.Fatal(err)
	}
	if got := string(sink.writes[0].(buffer.ByteBuf).Bytes()); got != "MAIL FROM:<a@example.com> SIZE=5\r\n" {
		t.Fatalf("line=%q", got)
	}
}

func TestResponseEncoderWritesMultilineResponse(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("smtp", NewResponseEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	if err := ch.Write(NewResponse(250, "mail.example", "PIPELINING")); err != nil {
		t.Fatal(err)
	}
	if got := string(sink.writes[0].(buffer.ByteBuf).Bytes()); got != "250-mail.example\r\n250 PIPELINING\r\n" {
		t.Fatalf("response=%q", got)
	}
}

func TestDataEncoderDotStuffsAndWritesTerminator(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("smtp", NewDataEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	if err := ch.Write(LastData(testBuf(".first\r\nsecond\r\n.third\r\n"))); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	if got := string(sink.writes[0].(buffer.ByteBuf).Bytes()); got != "..first\r\nsecond\r\n..third\r\n" {
		t.Fatalf("data=%q", got)
	}
	if got := string(sink.writes[1].(buffer.ByteBuf).Bytes()); got != "\r\n.\r\n" {
		t.Fatalf("tail=%q", got)
	}
}

func BenchmarkResponseDecoder(b *testing.B) {
	decoder, err := NewResponseDecoder(256)
	if err != nil {
		b.Fatal(err)
	}
	payload := "250-mail.example\r\n250-PIPELINING\r\n250 SIZE 1024\r\n"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		comp := singleComposite(testBuf(payload))
		out, err := decoder.Decode(nil, comp)
		if err != nil {
			b.Fatal(err)
		}
		if out != nil {
			b.Fatal("first multiline response line should not emit")
		}
		out, err = decoder.Decode(nil, comp)
		if err != nil {
			b.Fatal(err)
		}
		if out != nil {
			b.Fatal("second multiline response line should not emit")
		}
		out, err = decoder.Decode(nil, comp)
		if err != nil {
			b.Fatal(err)
		}
		if out == nil {
			b.Fatal("final response line should emit")
		}
		comp.Release()
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

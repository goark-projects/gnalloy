package rtsp

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestRequestDecoderParsesBodyZeroCopy(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	decoder, err := NewRequestDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("rtsp", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	in := testBuf("ANNOUNCE rtsp://example/media RTSP/1.0\r\nCSeq: 2\r\nContent-Length: 4\r\n\r\nbody")
	ch.Pipeline().FireChannelRead(in)
	req := collector.msgs[0].(Request)
	defer req.Release()
	if req.Method != MethodAnnounce || req.URI != "rtsp://example/media" || req.Headers.Get("CSeq") != "2" {
		t.Fatalf("req=%+v", req)
	}
	if string(req.Body.Bytes()) != "body" {
		t.Fatalf("body=%q", req.Body.Bytes())
	}
	if in.RefCnt() != 1 {
		t.Fatalf("input ref=%d, want 1 while body slice is alive", in.RefCnt())
	}
}

func TestResponseDecoderParsesResponse(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	decoder, err := NewResponseDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("rtsp", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(testBuf("RTSP/1.0 200 OK\r\nCSeq: 2\r\n\r\n"))
	resp := collector.msgs[0].(Response)
	if resp.StatusCode != 200 || resp.Headers.Get("cseq") != "2" {
		t.Fatalf("resp=%+v", resp)
	}
}

func TestRequestEncoderWritesContentLengthAndBody(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("rtsp", NewRequestEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	body := testBuf("sdp")
	if err := ch.Write(Request{Method: MethodDescribe, URI: "rtsp://example/media", Headers: Headers{"CSeq": "1"}, Body: body}); err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(sink.writes))
	}
	header := string(sink.writes[0].(buffer.ByteBuf).Bytes())
	if header != "DESCRIBE rtsp://example/media RTSP/1.0\r\nCSeq: 1\r\nContent-Length: 3\r\n\r\n" {
		t.Fatalf("header=%q", header)
	}
	if sink.writes[1] != body {
		t.Fatal("body should pass through without copy")
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

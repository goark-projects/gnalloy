package main

import (
	"bytes"
	"testing"

	"goark.dev/gnalloy/benchmarks/external/internal/benchhttp"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestHTTP1RawHandlerWritesFixedResponse(t *testing.T) {
	sink := &rawHTTP1Sink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("raw", newHTTP1RawHandler(128)); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(rawHTTP1Buf(benchhttp.RequestBytes("127.0.0.1")))

	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	want := benchhttp.ResponseBytes(128)
	if !bytes.Equal(sink.writes[0], want) {
		t.Fatalf("response len=%d, want %d", len(sink.writes[0]), len(want))
	}
	if sink.flushes != 1 {
		t.Fatalf("flushes=%d, want 1", sink.flushes)
	}
}

func TestHTTP1RawHandlerHandlesFragmentedRequest(t *testing.T) {
	sink := &rawHTTP1Sink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("raw", newHTTP1RawHandler(16)); err != nil {
		t.Fatal(err)
	}

	req := benchhttp.RequestBytes("127.0.0.1")
	ch.Pipeline().FireChannelRead(rawHTTP1Buf(req[:len(req)-2]))
	if len(sink.writes) != 0 {
		t.Fatalf("writes=%d before request complete", len(sink.writes))
	}
	ch.Pipeline().FireChannelRead(rawHTTP1Buf(req[len(req)-2:]))

	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	if !bytes.Equal(sink.writes[0], benchhttp.ResponseBytes(16)) {
		t.Fatal("unexpected response")
	}
}

func TestHTTP1RawHandlerBatchesResponsesFromOneRead(t *testing.T) {
	sink := &rawHTTP1Sink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("raw", newHTTP1RawHandler(32)); err != nil {
		t.Fatal(err)
	}

	req := benchhttp.RequestBytes("127.0.0.1")
	ch.Pipeline().FireChannelRead(rawHTTP1Buf(append(req, req...)))

	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	want := append(benchhttp.ResponseBytes(32), benchhttp.ResponseBytes(32)...)
	if !bytes.Equal(sink.writes[0], want) {
		t.Fatalf("batched response len=%d, want %d", len(sink.writes[0]), len(want))
	}
	if sink.flushes != 1 {
		t.Fatalf("flushes=%d, want 1", sink.flushes)
	}
}

type rawHTTP1Sink struct {
	writes  [][]byte
	flushes int
}

func (s *rawHTTP1Sink) Write(msg any) error {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return nil
	}
	s.writes = append(s.writes, append([]byte(nil), buf.Bytes()...))
	buf.Release()
	return nil
}

func (s *rawHTTP1Sink) Flush() error {
	s.flushes++
	return nil
}

func (s *rawHTTP1Sink) Close() error { return nil }

func rawHTTP1Buf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

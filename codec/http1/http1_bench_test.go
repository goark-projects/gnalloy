package http1

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func BenchmarkFindHeaderEndFragmented(b *testing.B) {
	in := fragmentedHTTP1Buffer(
		"GET /bench HTTP/1.1\r\n",
		"Host: example.test\r\n",
		"User-Agent: gnalloy\r\n",
		"Content-Length: 0\r\n",
		"\r\n",
	)
	defer in.Release()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if index, ok := findHeaderEnd(in); !ok || index != in.WriterIndex()-4 {
			b.Fatalf("index=%d ok=%v", index, ok)
		}
	}
}

func BenchmarkStringSliceFragmented(b *testing.B) {
	in := fragmentedHTTP1Buffer("GET /bench HTTP/1.1\r\n", "Host: example.test\r\n", "\r\n")
	defer in.Release()
	length := in.ReadableBytes()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if value, err := stringSlice(in, in.ReaderIndex(), length); err != nil || len(value) != length {
			b.Fatalf("len=%d err=%v", len(value), err)
		}
	}
}

func BenchmarkRequestDecoderFragmentedHeader(b *testing.B) {
	decoder, err := NewRequestDecoder(1024, 1024)
	if err != nil {
		b.Fatal(err)
	}
	collector := &requestCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch.Pipeline().FireChannelRead(testBuf([]byte("GET /bench HTTP/1.1\r\nHost: example.test\r\n")))
		ch.Pipeline().FireChannelRead(testBuf([]byte("Content-Length: 0\r\n\r\n")))
		if len(collector.reqs) != 1 {
			b.Fatalf("reqs=%d", len(collector.reqs))
		}
		collector.reqs = collector.reqs[:0]
	}
}

func BenchmarkRequestDecoderChunkedFragmentedBody(b *testing.B) {
	decoder, err := NewRequestDecoder(1024, 1024)
	if err != nil {
		b.Fatal(err)
	}
	collector := &requestCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch.Pipeline().FireChannelRead(testBuf([]byte("POST /bench HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nab")))
		ch.Pipeline().FireChannelRead(testBuf([]byte("cd\r\n4\r\nefgh\r\n0\r\n\r\n")))
		if len(collector.reqs) != 1 {
			b.Fatalf("reqs=%d", len(collector.reqs))
		}
		collector.reqs[0].Release()
		collector.reqs = collector.reqs[:0]
	}
}

func fragmentedHTTP1Buffer(parts ...string) *buffer.CompositeByteBuf {
	c := buffer.NewCompositeByteBuf()
	for _, part := range parts {
		c.Append(testBuf([]byte(part)))
	}
	return c
}

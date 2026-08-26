package http1

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

const maxHTTP1FuzzInput = 4096

func FuzzHTTP1RequestDecoder(f *testing.F) {
	f.Add([]byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"))
	f.Add([]byte("POST /x HTTP/1.1\r\nContent-Length: 5\r\n\r\nhello"))
	f.Add([]byte("POST /chunk HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nWiki\r\n0\r\n\r\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		decoder, err := NewRequestDecoder(2048, 2048)
		if err != nil {
			t.Fatal(err)
		}
		fuzzHTTP1ByteDecoder(t, decoder, data)
	})
}

func FuzzHTTP1ResponseDecoder(f *testing.F) {
	f.Add([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
	f.Add([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
	f.Add([]byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n2\r\nok\r\n0\r\n\r\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		decoder, err := NewResponseDecoder(2048, 2048)
		if err != nil {
			t.Fatal(err)
		}
		fuzzHTTP1ByteDecoder(t, decoder, data)
	})
}

func FuzzHTTP1ObjectRequestDecoder(f *testing.F) {
	f.Add([]byte("GET / HTTP/1.1\r\n\r\n"))
	f.Add([]byte("POST /stream HTTP/1.1\r\nContent-Length: 4\r\n\r\nbody"))
	f.Fuzz(func(t *testing.T, data []byte) {
		decoder, err := NewRequestObjectDecoder(2048, 2048)
		if err != nil {
			t.Fatal(err)
		}
		fuzzHTTP1ByteDecoder(t, decoder, data)
	})
}

func FuzzHTTP1ObjectResponseDecoder(f *testing.F) {
	f.Add([]byte("HTTP/1.1 204 No Content\r\n\r\n"))
	f.Add([]byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\nbody"))
	f.Fuzz(func(t *testing.T, data []byte) {
		decoder, err := NewResponseObjectDecoder(2048, 2048)
		if err != nil {
			t.Fatal(err)
		}
		fuzzHTTP1ByteDecoder(t, decoder, data)
	})
}

func fuzzHTTP1ByteDecoder(t *testing.T, decoder channel.Handler, data []byte) {
	t.Helper()
	if len(data) > maxHTTP1FuzzInput {
		return
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("decoder", decoder); err != nil {
		t.Fatal(err)
	}
	fireHTTP1FuzzBytes(ch, data)
	ch.Pipeline().FireChannelInactive()
}

func fireHTTP1FuzzBytes(ch channel.Channel, data []byte) {
	if len(data) == 0 {
		ch.Pipeline().FireChannelRead(http1FuzzByteBuf(nil))
		return
	}
	mid := len(data) / 2
	if mid > 0 {
		ch.Pipeline().FireChannelRead(http1FuzzByteBuf(data[:mid]))
	}
	ch.Pipeline().FireChannelRead(http1FuzzByteBuf(data[mid:]))
}

func http1FuzzByteBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

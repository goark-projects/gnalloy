package redis

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

const maxRedisFuzzInput = 4096

func FuzzRedisFramePipeline(f *testing.F) {
	f.Add([]byte("+OK\r\n"))
	f.Add([]byte(":7\r\n"))
	f.Add([]byte("$5\r\nhello\r\n"))
	f.Add([]byte("*2\r\n$4\r\nPING\r\n$1\r\nx\r\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxRedisFuzzInput {
			return
		}
		frameDecoder, err := NewFrameDecoder(1024)
		if err != nil {
			t.Fatal(err)
		}
		ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
		if err := ch.Pipeline().AddLast("frame", frameDecoder); err != nil {
			t.Fatal(err)
		}
		if err := ch.Pipeline().AddLast("value", NewValueDecoder()); err != nil {
			t.Fatal(err)
		}
		fireRedisFuzzBytes(ch, data)
		ch.Pipeline().FireChannelInactive()
	})
}

func fireRedisFuzzBytes(ch channel.Channel, data []byte) {
	if len(data) == 0 {
		ch.Pipeline().FireChannelRead(redisFuzzByteBuf(nil))
		return
	}
	mid := len(data) / 2
	if mid > 0 {
		ch.Pipeline().FireChannelRead(redisFuzzByteBuf(data[:mid]))
	}
	ch.Pipeline().FireChannelRead(redisFuzzByteBuf(data[mid:]))
}

func redisFuzzByteBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

package http3

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

const maxHTTP3FuzzInput = 4096

func FuzzHTTP3FramePipeline(f *testing.F) {
	f.Add([]byte{byte(FrameData), 5, 'h', 'e', 'l', 'l', 'o'})
	f.Add([]byte{byte(FrameHeaders), 3, 'h', 'd', 'r'})
	f.Add([]byte{byte(FrameSettings), 2, 1, 10})
	f.Add([]byte{0x21, 3, 'x', 'y', 'z'})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxHTTP3FuzzInput {
			return
		}
		decoder, err := NewDecoder(1024)
		if err != nil {
			t.Fatal(err)
		}
		ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
		if err := ch.Pipeline().AddLast("http3", decoder); err != nil {
			t.Fatal(err)
		}
		fireHTTP3FuzzBytes(ch, data)
		ch.Pipeline().FireChannelInactive()
	})
}

func fireHTTP3FuzzBytes(ch channel.Channel, data []byte) {
	if len(data) == 0 {
		ch.Pipeline().FireChannelRead(http3FuzzByteBuf(nil))
		return
	}
	mid := len(data) / 2
	if mid > 0 {
		ch.Pipeline().FireChannelRead(http3FuzzByteBuf(data[:mid]))
	}
	ch.Pipeline().FireChannelRead(http3FuzzByteBuf(data[mid:]))
}

func http3FuzzByteBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

package http2

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

const maxHTTP2FuzzInput = 4096

func FuzzHTTP2FramePipeline(f *testing.F) {
	f.Add([]byte{0, 0, 4, byte(FrameData), byte(FlagEndStream), 0, 0, 0, 1, 'p', 'i', 'n', 'g'})
	f.Add([]byte{0, 0, 0, byte(FrameSettings), 0, 0, 0, 0, 0})
	f.Add([]byte{0, 0, 8, byte(FramePing), 0, 0, 0, 0, 0, '1', '2', '3', '4', '5', '6', '7', '8'})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxHTTP2FuzzInput {
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
		if err := ch.Pipeline().AddLast("typed", NewTypedFrameDecoder()); err != nil {
			t.Fatal(err)
		}
		fireHTTP2FuzzBytes(ch, data)
		ch.Pipeline().FireChannelInactive()
	})
}

func fireHTTP2FuzzBytes(ch channel.Channel, data []byte) {
	if len(data) == 0 {
		ch.Pipeline().FireChannelRead(http2FuzzByteBuf(nil))
		return
	}
	mid := len(data) / 2
	if mid > 0 {
		ch.Pipeline().FireChannelRead(http2FuzzByteBuf(data[:mid]))
	}
	ch.Pipeline().FireChannelRead(http2FuzzByteBuf(data[mid:]))
}

func http2FuzzByteBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

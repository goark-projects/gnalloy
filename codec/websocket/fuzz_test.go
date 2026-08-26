package websocket

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

const maxWebSocketFuzzInput = 4096

func FuzzWebSocketFrameDecoder(f *testing.F) {
	f.Add([]byte{0x81, 0x02, 'o', 'k'})
	f.Add([]byte{0x81, 0x82, 1, 2, 3, 4, 'h' ^ 1, 'i' ^ 2})
	f.Add([]byte{0x88, 0x02, 0x03, 0xe8})
	f.Add([]byte{0x89, 0x7e, 0, 126})
	f.Fuzz(func(t *testing.T, data []byte) {
		decoder, err := NewFrameDecoderWithMaskPolicy(1024, false, true)
		if err != nil {
			t.Fatal(err)
		}
		fuzzWebSocketByteDecoder(t, decoder, data)
	})
}

func fuzzWebSocketByteDecoder(t *testing.T, decoder channel.Handler, data []byte) {
	t.Helper()
	if len(data) > maxWebSocketFuzzInput {
		return
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("decoder", decoder); err != nil {
		t.Fatal(err)
	}
	fireWebSocketFuzzBytes(ch, data)
	ch.Pipeline().FireChannelInactive()
}

func fireWebSocketFuzzBytes(ch channel.Channel, data []byte) {
	if len(data) == 0 {
		ch.Pipeline().FireChannelRead(webSocketFuzzByteBuf(nil))
		return
	}
	mid := len(data) / 2
	if mid > 0 {
		ch.Pipeline().FireChannelRead(webSocketFuzzByteBuf(data[:mid]))
	}
	ch.Pipeline().FireChannelRead(webSocketFuzzByteBuf(data[mid:]))
}

func webSocketFuzzByteBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}

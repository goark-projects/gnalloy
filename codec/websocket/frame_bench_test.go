package websocket

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func BenchmarkFrameEncoderMaskedCompositePayload(b *testing.B) {
	sink := &outboundSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	_ = ch.Pipeline().AddLast("encoder", NewFrameEncoder())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ch.Write(Frame{
			Final:   true,
			Opcode:  OpcodeBinary,
			Payload: fragmentedWebSocketPayload("abcd", "efgh", "ijkl", "mnop"),
			Masked:  true,
			MaskKey: [4]byte{1, 2, 3, 4},
		}); err != nil {
			b.Fatal(err)
		}
		sink.release()
		sink.writes = sink.writes[:0]
	}
}

func BenchmarkFrameDecoderMaskedFragmentedPayload(b *testing.B) {
	decoder, err := NewFrameDecoder(1024)
	if err != nil {
		b.Fatal(err)
	}
	collector := &frameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, part := range [][]byte{
			{0x82, 0x90, 1, 2, 3, 4},
			maskedPayloadBytes("abcd", [4]byte{1, 2, 3, 4}),
			maskedPayloadBytes("efgh", [4]byte{1, 2, 3, 4}),
			maskedPayloadBytes("ijkl", [4]byte{1, 2, 3, 4}),
			maskedPayloadBytes("mnop", [4]byte{1, 2, 3, 4}),
		} {
			ch.Pipeline().FireChannelRead(testBuf(part))
		}
		if len(collector.frames) != 1 || collector.frames[0].Payload.ReadableBytes() != 16 {
			b.Fatalf("frames=%d", len(collector.frames))
		}
		for _, frame := range collector.frames {
			frame.Release()
		}
		collector.frames = collector.frames[:0]
	}
}

func fragmentedWebSocketPayload(parts ...string) buffer.ByteBuf {
	c := buffer.NewCompositeByteBuf()
	for _, part := range parts {
		c.Append(testBuf([]byte(part)))
	}
	return c
}

func maskedPayloadBytes(value string, key [4]byte) []byte {
	out := []byte(value)
	for i := range out {
		out[i] ^= key[i&3]
	}
	return out
}

package codec

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func BenchmarkFixedLengthFrameDecoder(b *testing.B) {
	decoder, err := NewFixedLengthFrameDecoder(4)
	if err != nil {
		b.Fatal(err)
	}
	benchmarkFrameDecoder(b, decoder, []byte("ping"))
}

func BenchmarkLineBasedFrameDecoder(b *testing.B) {
	decoder, err := NewLineBasedFrameDecoder(64)
	if err != nil {
		b.Fatal(err)
	}
	benchmarkFrameDecoder(b, decoder, []byte("ping\n"))
}

func BenchmarkDelimiterBasedFrameDecoder(b *testing.B) {
	decoder, err := NewDelimiterBasedFrameDecoder(64, true, true, []byte("<END>"))
	if err != nil {
		b.Fatal(err)
	}
	benchmarkFrameDecoder(b, decoder, []byte("ping<END>"))
}

func BenchmarkByteToMessageListDecoder(b *testing.B) {
	decoder := NewByteToMessageListDecoder(byteListDecoderFunc{
		decode: func(_ *channel.HandlerContext, in *buffer.CompositeByteBuf, out *MessageList) error {
			for in.ReadableBytes() >= 4 {
				frame, err := in.Slice(in.ReaderIndex(), 4)
				if err != nil {
					return err
				}
				if err := in.SkipBytes(4); err != nil {
					frame.Release()
					return err
				}
				out.Add(frame)
			}
			return nil
		},
	})
	benchmarkFrameDecoder(b, decoder, []byte("pingpong"))
}

func benchmarkFrameDecoder(b *testing.B, decoder channel.Handler, payload []byte) {
	collector := &frameCollector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	_ = ch.Pipeline().AddLast("decoder", decoder)
	_ = ch.Pipeline().AddLast("collector", collector)
	alloc := buffer.NewHeapAllocator()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf, err := alloc.Acquire(len(payload))
		if err != nil {
			b.Fatal(err)
		}
		_, _ = buf.WriteBytes(payload)
		ch.Pipeline().FireChannelRead(buf)
		collector.release()
	}
}

package quic

import (
	"testing"

	"goark.dev/gnalloy/buffer"
)

func BenchmarkFrameScanner(b *testing.B) {
	cryptoPayload := quicTestBuf("crypto-data")
	streamPayload := quicTestBuf("stream-data")
	encoded, err := AppendFrame(nil, PingFrame{})
	if err != nil {
		b.Fatal(err)
	}
	encoded, err = AppendFrame(encoded, CryptoFrame{Data: cryptoPayload})
	if err != nil {
		b.Fatal(err)
	}
	encoded, err = AppendFrame(encoded, StreamFrame{StreamID: 1, Data: streamPayload})
	if err != nil {
		b.Fatal(err)
	}
	cryptoPayload.Release()
	streamPayload.Release()
	payload := buffer.NewHeapBuffer(len(encoded))
	_, _ = payload.WriteBytes(encoded)
	defer payload.Release()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scanner := NewFrameScanner(payload)
		for {
			frame, ok, err := scanner.Next()
			if err != nil {
				b.Fatal(err)
			}
			if !ok {
				break
			}
			releaseFrame(frame)
		}
	}
}

func BenchmarkEncodeFrames(b *testing.B) {
	alloc := buffer.NewHeapAllocator()
	cryptoPayload := quicTestBuf("crypto-data")
	streamPayload := quicTestBuf("stream-data")
	defer cryptoPayload.Release()
	defer streamPayload.Release()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := EncodeFrames(alloc,
			PingFrame{},
			CryptoFrame{Data: cryptoPayload},
			StreamFrame{StreamID: 1, Data: streamPayload},
		)
		if err != nil {
			b.Fatal(err)
		}
		out.Release()
	}
}

package ip

import (
	"testing"

	"goark.dev/gnalloy/buffer"
)

func BenchmarkEncodeDecodeIPv4Packet(b *testing.B) {
	alloc := buffer.NewHeapAllocator()
	payload := testBuf("custom-payload")
	packet := Packet{
		Header:  Header{Version: Version4, Protocol: 253, Source: ipv4Src, Destination: ipv4Dst},
		Payload: payload,
	}
	encoded, err := EncodePacket(alloc, packet)
	if err != nil {
		b.Fatal(err)
	}
	defer encoded.Release()
	payload.Release()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decoded, err := DecodePacket(encoded)
		if err != nil {
			b.Fatal(err)
		}
		decoded.Release()
	}
}

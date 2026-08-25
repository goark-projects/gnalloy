package udp

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

func BenchmarkEndpointBackpressureQueue(b *testing.B) {
	ep := &endpoint{}
	ep.initBackpressure(transport.WriteBufferWatermark{Low: 32 * 1024, High: 64 * 1024})
	alloc := buffer.NewHeapAllocator()
	var payload [64]byte

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf, err := alloc.Acquire(64)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = buf.WriteBytes(payload[:])
		ep.enqueue(Datagram{Payload: buf, Addr: testAddr})
		ep.dequeue()
	}
}

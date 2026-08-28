package channel

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

func BenchmarkUnsafeWriteAndFlushDrained(b *testing.B) {
	alloc := buffer.NewHeapAllocator()
	rw := &fullWriteRW{}
	ch, _ := NewUnsafeChannel(UnsafeConfig{
		ID:         1,
		FD:         transport.FDRef{FD: 1},
		Allocator:  alloc,
		Poller:     &fakeReadyPoller{},
		ReadWriter: rw,
	})
	payload := make([]byte, 64)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf, err := alloc.Acquire(len(payload))
		if err != nil {
			b.Fatal(err)
		}
		payload[0] = byte(i)
		if _, err := buf.WriteBytes(payload); err != nil {
			b.Fatal(err)
		}
		if err := ch.WriteAndFlush(buf); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()

	if pending := ch.PendingOutboundBytes(); pending != 0 {
		b.Fatalf("pending outbound bytes=%d, want 0", pending)
	}
	if rw.writes != b.N {
		b.Fatalf("writes=%d, want %d", rw.writes, b.N)
	}
}

package channel

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

type benchVectorWriteRW struct {
	scalar int
	writev int
}

func (rw *benchVectorWriteRW) Read(transport.FDRef, []byte) (int, bool, error) {
	return 0, true, nil
}

func (rw *benchVectorWriteRW) Write(_ transport.FDRef, src []byte) (int, bool, error) {
	rw.scalar++
	return len(src), false, nil
}

func (rw *benchVectorWriteRW) Writev(_ transport.FDRef, src [][]byte) (int, bool, error) {
	rw.writev++
	total := 0
	for _, part := range src {
		total += len(part)
	}
	return total, false, nil
}

func (rw *benchVectorWriteRW) Close(transport.FDRef) error {
	return nil
}

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

func BenchmarkUnsafeVectorWriteAndFlushSingleDirectBufferDrained(b *testing.B) {
	alloc := buffer.NewHeapAllocator()
	rw := &benchVectorWriteRW{}
	ch, unsafeCh := NewUnsafeChannel(UnsafeConfig{
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
	if rw.scalar != b.N || rw.writev != 0 {
		b.Fatalf("scalar=%d writev=%d, want %d/0", rw.scalar, rw.writev, b.N)
	}
	if len(unsafeCh.writeSlices) != 0 {
		b.Fatalf("write slices=%d, want 0", len(unsafeCh.writeSlices))
	}
}

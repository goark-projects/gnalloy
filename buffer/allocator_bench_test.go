package buffer

import "testing"

func BenchmarkHeapAllocatorAcquireRelease(b *testing.B) {
	alloc := NewHeapAllocator()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf, err := alloc.Acquire(4096)
		if err != nil {
			b.Fatal(err)
		}
		buf.Release()
	}
}

func BenchmarkMmapAllocatorAcquireRelease(b *testing.B) {
	alloc, err := NewMmapAllocator(MmapAllocatorConfig{
		BlockSize: 4096,
		Blocks:    4096,
	})
	if err != nil {
		b.Skip(err)
	}
	defer alloc.Close()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf, err := alloc.Acquire(4096)
		if err != nil {
			b.Fatal(err)
		}
		buf.Release()
	}
}

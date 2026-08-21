//go:build linux

package buffer

import (
	"errors"
	"testing"
)

func TestMmapAllocatorRejectsInvalidAndOverflowSizes(t *testing.T) {
	if _, err := NewMmapAllocator(MmapAllocatorConfig{}); !errors.Is(err, ErrInvalidSize) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidSize)
	}
	if _, err := NewMmapAllocator(MmapAllocatorConfig{BlockSize: maxInt, Blocks: 2}); !errors.Is(err, ErrInvalidSize) {
		t.Fatalf("overflow err=%v, want %v", err, ErrInvalidSize)
	}
}

func TestMmapAllocatorCloseRejectsInUseBuffers(t *testing.T) {
	alloc, err := NewMmapAllocator(MmapAllocatorConfig{BlockSize: 1024, Blocks: 1})
	if err != nil {
		t.Fatal(err)
	}
	buf, err := alloc.Acquire(128)
	if err != nil {
		t.Fatal(err)
	}
	if err := alloc.Close(); !errors.Is(err, ErrAllocatorInUse) {
		t.Fatalf("close err=%v, want %v", err, ErrAllocatorInUse)
	}
	buf.Release()
	if err := alloc.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := alloc.Acquire(128); !errors.Is(err, ErrAllocatorClosed) {
		t.Fatalf("acquire err=%v, want %v", err, ErrAllocatorClosed)
	}
}

func TestMmapAllocatorStatsTracksInUseAndClosed(t *testing.T) {
	alloc, err := NewMmapAllocator(MmapAllocatorConfig{BlockSize: 1024, Blocks: 2})
	if err != nil {
		t.Fatal(err)
	}
	statsAlloc := alloc.(StatAllocator)
	if got := statsAlloc.Stats(); got.BlockSize != 1024 || got.Blocks != 2 || got.Free != 2 || got.InUse != 0 || !got.OffHeap {
		t.Fatalf("initial stats=%+v", got)
	}
	buf, err := alloc.Acquire(128)
	if err != nil {
		t.Fatal(err)
	}
	if got := statsAlloc.Stats(); got.Free != 1 || got.InUse != 1 || got.Closed {
		t.Fatalf("acquired stats=%+v", got)
	}
	buf.Release()
	if got := statsAlloc.Stats(); got.Free != 2 || got.InUse != 0 {
		t.Fatalf("released stats=%+v", got)
	}
	if err := alloc.Close(); err != nil {
		t.Fatal(err)
	}
	if got := statsAlloc.Stats(); !got.Closed || got.Blocks != 0 || got.Free != 0 {
		t.Fatalf("closed stats=%+v", got)
	}
}

func TestMmapAllocatorDirectDoubleReleaseDoesNotCorruptFreeList(t *testing.T) {
	alloc, err := NewMmapAllocator(MmapAllocatorConfig{BlockSize: 1024, Blocks: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer alloc.Close()

	buf, err := alloc.Acquire(128)
	if err != nil {
		t.Fatal(err)
	}
	direct := buf.(*DirectByteBuf)
	alloc.Release(direct)
	alloc.Release(direct)

	first, err := alloc.Acquire(128)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := alloc.Acquire(128); !errors.Is(err, ErrAllocatorExhausted) {
		t.Fatalf("second acquire err=%v, want %v", err, ErrAllocatorExhausted)
	}
	first.Release()
}

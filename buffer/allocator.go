package buffer

import "sync"

// Allocator 为 Channel 提供 ByteBuf，具体实现可来自堆、slab 或 mmap。
type Allocator interface {
	Acquire(size int) (ByteBuf, error)
	Release(buf *DirectByteBuf)
	Close() error
}

// HeapAllocator 是跨平台的基础分配器，用于测试、调试和通用场景。
type HeapAllocator struct {
	pool sync.Pool
}

func NewHeapAllocator() *HeapAllocator {
	return &HeapAllocator{}
}

func NewHeapBuffer(size int) *DirectByteBuf {
	if size < 0 {
		size = 0
	}
	return newDirectByteBuf(make([]byte, size), nil)
}

func (a *HeapAllocator) Acquire(size int) (ByteBuf, error) {
	if size <= 0 {
		return nil, ErrInvalidSize
	}
	if v := a.pool.Get(); v != nil {
		buf := v.(*DirectByteBuf)
		if cap(buf.data) >= size {
			buf.reset(buf.data[:size], a)
			return buf, nil
		}
	}
	return newDirectByteBuf(make([]byte, size), a), nil
}

func (a *HeapAllocator) Release(buf *DirectByteBuf) {
	if buf == nil {
		return
	}
	buf.readerIndex = 0
	buf.writerIndex = 0
	buf.owner = a
	a.pool.Put(buf)
}

func (a *HeapAllocator) releaseDirect(buf *DirectByteBuf) {
	a.Release(buf)
}

func (a *HeapAllocator) Close() error {
	return nil
}

// MmapAllocatorConfig 描述 Linux mmap allocator 的静态容量配置。
type MmapAllocatorConfig struct {
	BlockSize int
	Blocks    int
}

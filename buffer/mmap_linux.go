//go:build linux

package buffer

import "golang.org/x/sys/unix"

type mmapAllocator struct {
	data      []byte
	blockSize int
	buffers   []DirectByteBuf
	freeList  []uint32
	freeTop   int
	closed    bool
}

// NewMmapAllocator 创建 Linux mmap-backed allocator。
// 该分配器面向单 EventLoop 使用，payload 来自 mmap，分配路径无锁、无 make。
func NewMmapAllocator(cfg MmapAllocatorConfig) (Allocator, error) {
	if cfg.BlockSize <= 0 || cfg.Blocks <= 0 {
		return nil, ErrInvalidSize
	}
	data, err := unix.Mmap(-1, 0, cfg.BlockSize*cfg.Blocks, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_ANONYMOUS|unix.MAP_PRIVATE)
	if err != nil {
		return nil, err
	}
	a := &mmapAllocator{
		data:      data,
		blockSize: cfg.BlockSize,
		buffers:   make([]DirectByteBuf, cfg.Blocks),
		freeList:  make([]uint32, cfg.Blocks),
		freeTop:   cfg.Blocks - 1,
	}
	for i := 0; i < cfg.Blocks; i++ {
		a.freeList[i] = uint32(i)
		start := i * cfg.BlockSize
		a.buffers[i].data = a.data[start : start+cfg.BlockSize]
		a.buffers[i].owner = a
		a.buffers[i].ownerIndex = uint32(i)
	}
	return a, nil
}

func (a *mmapAllocator) Acquire(size int) (ByteBuf, error) {
	if a.closed {
		return nil, ErrAllocatorClosed
	}
	if size <= 0 || size > a.blockSize {
		return nil, ErrInvalidSize
	}
	if a.freeTop < 0 {
		return nil, ErrAllocatorExhausted
	}
	idx := a.freeList[a.freeTop]
	a.freeTop--
	buf := &a.buffers[idx]
	buf.reset(buf.data[:a.blockSize], a)
	buf.ownerIndex = idx
	return buf, nil
}

func (a *mmapAllocator) Release(buf *DirectByteBuf) {
	if buf == nil || a.closed {
		return
	}
	idx := buf.ownerIndex
	if int(idx) >= len(a.buffers) || &a.buffers[idx] != buf {
		return
	}
	buf.readerIndex = 0
	buf.writerIndex = 0
	a.freeTop++
	a.freeList[a.freeTop] = idx
}

func (a *mmapAllocator) releaseDirect(buf *DirectByteBuf) {
	a.Release(buf)
}

func (a *mmapAllocator) Close() error {
	if a.data == nil {
		return nil
	}
	a.closed = true
	err := unix.Munmap(a.data)
	a.data = nil
	a.buffers = nil
	a.freeList = nil
	a.freeTop = -1
	return err
}

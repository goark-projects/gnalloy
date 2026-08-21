package tcp

import (
	"errors"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

// NewMmapAllocatorFactory 创建每个 Worker EventLoop 独占的 mmap allocator 工厂。
// fallbackToHeap 为 true 时，只有平台不支持 mmap 才回退到 HeapAllocator。
func NewMmapAllocatorFactory(cfg buffer.MmapAllocatorConfig, fallbackToHeap bool) AllocatorFactory {
	return func(*transport.EventLoop) (buffer.Allocator, error) {
		alloc, err := buffer.NewMmapAllocator(cfg)
		if err == nil {
			return alloc, nil
		}
		if fallbackToHeap && errors.Is(err, buffer.ErrUnsupportedMmap) {
			return buffer.NewHeapAllocator(), nil
		}
		return nil, err
	}
}

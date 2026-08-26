package tls

import "sync"

const (
	defaultBytePoolBufferSize = 16 * 1024
	defaultBytePoolMaxSize    = 64 * 1024
)

// BytePool 为 TLS handler 提供跨 goroutine 边界使用的临时字节切片。
//
// 实现必须并发安全。Acquire 返回的切片长度必须等于 size；Release 调用后调用方
// 不得再访问该切片。
type BytePool interface {
	Acquire(size int) []byte
	Release(buf []byte)
}

// BytePoolConfig 描述默认池化字节切片的容量策略。
type BytePoolConfig struct {
	// DefaultSize 是池中常规缓冲区容量，0 表示 16 KiB。
	DefaultSize int
	// MaxSize 是允许回收到池中的最大容量，0 表示 64 KiB。
	MaxSize int
	// ZeroOnRelease 控制释放时是否清零内容；默认关闭以保护热路径性能。
	ZeroOnRelease bool
}

// PooledBytePool 是基于 sync.Pool 的 TLS 临时字节池。
type PooledBytePool struct {
	pool          sync.Pool
	defaultSize   int
	maxSize       int
	zeroOnRelease bool
}

var defaultBytePool = NewPooledBytePool(BytePoolConfig{})

// NewPooledBytePool 创建可复用的 TLS 临时字节池。
func NewPooledBytePool(cfg BytePoolConfig) *PooledBytePool {
	defaultSize := cfg.DefaultSize
	if defaultSize <= 0 {
		defaultSize = defaultBytePoolBufferSize
	}
	maxSize := cfg.MaxSize
	if maxSize <= 0 {
		maxSize = defaultBytePoolMaxSize
	}
	if maxSize < defaultSize {
		maxSize = defaultSize
	}
	p := &PooledBytePool{
		defaultSize:   defaultSize,
		maxSize:       maxSize,
		zeroOnRelease: cfg.ZeroOnRelease,
	}
	p.pool.New = func() any {
		return make([]byte, defaultSize)
	}
	return p
}

// Acquire 返回长度为 size 的临时切片。
func (p *PooledBytePool) Acquire(size int) []byte {
	if size <= 0 {
		return nil
	}
	if p == nil {
		return make([]byte, size)
	}
	buf := p.pool.Get().([]byte)
	if cap(buf) < size {
		return make([]byte, size)
	}
	return buf[:size]
}

// Release 回收临时切片。
func (p *PooledBytePool) Release(buf []byte) {
	if p == nil || cap(buf) == 0 || cap(buf) > p.maxSize {
		return
	}
	if p.zeroOnRelease {
		clear(buf[:cap(buf)])
	}
	size := p.defaultSize
	if cap(buf) < size {
		size = cap(buf)
	}
	p.pool.Put(buf[:size])
}

func normalizeBytePool(pool BytePool) BytePool {
	if pool == nil {
		return defaultBytePool
	}
	return pool
}

func acquireBytes(pool BytePool, size int) []byte {
	if size <= 0 {
		return nil
	}
	if pool == nil {
		return make([]byte, size)
	}
	return pool.Acquire(size)
}

func releaseBytes(pool BytePool, buf []byte) {
	if pool == nil || buf == nil {
		return
	}
	pool.Release(buf)
}

type byteChunk struct {
	data    []byte
	owner   []byte
	release func([]byte)
}

func newByteChunk(data []byte, pool BytePool) byteChunk {
	if pool == nil {
		pool = defaultBytePool
	}
	return byteChunk{data: data, owner: data, release: pool.Release}
}

func (c *byteChunk) releaseOwned() {
	if c == nil || c.owner == nil || c.release == nil {
		return
	}
	c.release(c.owner)
	*c = byteChunk{}
}

package traffic

import (
	"sync"
	"time"
)

// ClockMillis 返回当前毫秒时间。
type ClockMillis func() int64

// Limiter 根据当前时间和字节数返回需要延迟的毫秒数。
type Limiter interface {
	Reserve(nowMillis int64, bytes int) int64
}

// Config 描述读写流量整形参数。
type Config struct {
	// ReadLimitBytesPerSecond 是入站读限速，0 表示不限速。
	ReadLimitBytesPerSecond int64
	// WriteLimitBytesPerSecond 是出站写限速，0 表示不限速。
	WriteLimitBytesPerSecond int64
	// MaxDelayMillis 是单次整形最大延迟，0 表示不截断延迟。
	MaxDelayMillis int64
	// Clock 注入毫秒时钟，用于测试和自定义运行时。
	Clock ClockMillis
}

// Controller 持有可共享的读写限速器和全局统计。
type Controller struct {
	readLimiter  Limiter
	writeLimiter Limiter
	clock        ClockMillis
	stats        counters
}

// NewController 创建可在多个 Channel Handler 间共享的限速控制器。
func NewController(cfg Config) (*Controller, error) {
	if cfg.ReadLimitBytesPerSecond < 0 || cfg.WriteLimitBytesPerSecond < 0 || cfg.MaxDelayMillis < 0 {
		return nil, ErrInvalidConfig
	}
	clock := cfg.Clock
	if clock == nil {
		clock = func() int64 { return time.Now().UnixMilli() }
	}
	return &Controller{
		readLimiter:  newRateLimiter(cfg.ReadLimitBytesPerSecond, cfg.MaxDelayMillis),
		writeLimiter: newRateLimiter(cfg.WriteLimitBytesPerSecond, cfg.MaxDelayMillis),
		clock:        clock,
	}, nil
}

// Stats 返回共享控制器的全局统计快照。
func (c *Controller) Stats() Snapshot {
	if c == nil {
		return Snapshot{}
	}
	return c.stats.snapshot(0, 0)
}

func (c *Controller) nowMillis() int64 {
	if c == nil || c.clock == nil {
		return time.Now().UnixMilli()
	}
	return c.clock()
}

func (c *Controller) reserveRead(bytes int) int64 {
	if c == nil {
		return 0
	}
	c.stats.readBytes.Add(int64(nonNegative(bytes)))
	if c.readLimiter == nil {
		return 0
	}
	delay := c.readLimiter.Reserve(c.nowMillis(), bytes)
	if delay > 0 {
		c.stats.delayedReads.Add(1)
	}
	return delay
}

func (c *Controller) reserveWrite(bytes int) int64 {
	if c == nil {
		return 0
	}
	c.stats.writtenBytes.Add(int64(nonNegative(bytes)))
	if c.writeLimiter == nil {
		return 0
	}
	delay := c.writeLimiter.Reserve(c.nowMillis(), bytes)
	if delay > 0 {
		c.stats.delayedWrites.Add(1)
	}
	return delay
}

type rateLimiter struct {
	bytesPerSecond int64
	maxDelayMillis int64

	mu                  sync.Mutex
	nextAvailableMillis int64
}

func newRateLimiter(bytesPerSecond int64, maxDelayMillis int64) Limiter {
	if bytesPerSecond <= 0 {
		return nil
	}
	return &rateLimiter{bytesPerSecond: bytesPerSecond, maxDelayMillis: maxDelayMillis}
}

func (l *rateLimiter) Reserve(nowMillis int64, bytes int) int64 {
	if l == nil || l.bytesPerSecond <= 0 || bytes <= 0 {
		return 0
	}
	cost := (int64(bytes)*1000 + l.bytesPerSecond - 1) / l.bytesPerSecond
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.nextAvailableMillis < nowMillis {
		l.nextAvailableMillis = nowMillis
	}
	delay := l.nextAvailableMillis - nowMillis
	l.nextAvailableMillis += cost
	if l.maxDelayMillis > 0 && delay > l.maxDelayMillis {
		return l.maxDelayMillis
	}
	return delay
}

func nonNegative(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

package dns

import (
	"net"
	"strings"
	"sync"
	"time"
)

// Cache 抽象 DNS 解析结果缓存。
//
// 实现必须并发安全；Lookup 返回的 IP 切片必须可由调用方独立修改。
type Cache interface {
	Lookup(host string, now time.Time) ([]net.IP, error, bool)
	Store(host string, ips []net.IP, err error, ttl time.Duration, now time.Time)
	Delete(host string)
}

// MemoryCache 是进程内 DNS 缓存，支持正向结果和负缓存。
type MemoryCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	ips       []net.IP
	err       error
	expiresAt time.Time
}

// NewMemoryCache 创建进程内 DNS 缓存。
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{entries: make(map[string]cacheEntry, 64)}
}

// Lookup 查询未过期缓存项。
func (c *MemoryCache) Lookup(host string, now time.Time) ([]net.IP, error, bool) {
	if c == nil {
		return nil, nil, false
	}
	key := normalizeCacheKey(host)
	if key == "" {
		return nil, nil, false
	}
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, nil, false
	}
	if !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
		c.Delete(host)
		return nil, nil, false
	}
	return cloneIPs(entry.ips), entry.err, true
}

// Store 写入带 TTL 的缓存项，ttl 小于等于 0 时忽略。
func (c *MemoryCache) Store(host string, ips []net.IP, err error, ttl time.Duration, now time.Time) {
	if c == nil || ttl <= 0 {
		return
	}
	key := normalizeCacheKey(host)
	if key == "" {
		return
	}
	entry := cacheEntry{
		ips:       cloneIPs(ips),
		err:       err,
		expiresAt: now.Add(ttl),
	}
	c.mu.Lock()
	c.entries[key] = entry
	c.mu.Unlock()
}

// Delete 删除指定 host 的缓存项。
func (c *MemoryCache) Delete(host string) {
	if c == nil {
		return
	}
	key := normalizeCacheKey(host)
	if key == "" {
		return
	}
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func normalizeCacheKey(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	return strings.TrimSuffix(host, ".")
}

func cloneIPs(ips []net.IP) []net.IP {
	if len(ips) == 0 {
		return nil
	}
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		out = append(out, append(net.IP(nil), ip...))
	}
	return out
}

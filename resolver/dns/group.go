package dns

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
)

// ResolverFactory 创建 resolver 实例。
type ResolverFactory func(Config) *Resolver

// ResolverGroup 按调用方 key 复用 Resolver。
//
// key 通常映射为 EventLoop ID、租户 ID 或上游集群名。该结构对齐 Netty
// DnsAddressResolverGroup 的生命周期语义：同一 key 返回同一 resolver，不同 key
// 隔离缓存和轮询状态。
type ResolverGroup[K comparable] struct {
	cfg      Config
	factory  ResolverFactory
	mu       sync.RWMutex
	resolver map[K]*Resolver
}

// NewResolverGroup 创建按 key 复用的 DNS resolver group。
func NewResolverGroup[K comparable](cfg Config) *ResolverGroup[K] {
	return NewResolverGroupWithFactory[K](cfg, NewResolver)
}

// NewResolverGroupWithFactory 使用自定义工厂创建 DNS resolver group。
func NewResolverGroupWithFactory[K comparable](cfg Config, factory ResolverFactory) *ResolverGroup[K] {
	if factory == nil {
		factory = NewResolver
	}
	return &ResolverGroup[K]{
		cfg:      cfg,
		factory:  factory,
		resolver: make(map[K]*Resolver, 8),
	}
}

// Resolver 返回 key 对应的 resolver；不存在时惰性创建。
func (g *ResolverGroup[K]) Resolver(key K) *Resolver {
	if g == nil {
		return NewResolver(Config{})
	}
	g.mu.RLock()
	resolver := g.resolver[key]
	g.mu.RUnlock()
	if resolver != nil {
		return resolver
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if resolver = g.resolver[key]; resolver != nil {
		return resolver
	}
	resolver = g.factory(g.cfg)
	g.resolver[key] = resolver
	return resolver
}

// Delete 删除 key 关联的 resolver。
func (g *ResolverGroup[K]) Delete(key K) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	_, ok := g.resolver[key]
	delete(g.resolver, key)
	g.mu.Unlock()
	return ok
}

// Clear 清空全部 resolver。
func (g *ResolverGroup[K]) Clear() {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.resolver = make(map[K]*Resolver, 8)
	g.mu.Unlock()
}

// Size 返回当前缓存的 resolver 数量。
func (g *ResolverGroup[K]) Size() int {
	if g == nil {
		return 0
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.resolver)
}

// RoundRobinResolver 在单次解析结果内做轮询排序。
type RoundRobinResolver struct {
	resolver *Resolver
	next     atomic.Uint64
}

// NewRoundRobinResolver 创建轮询排序 resolver。
func NewRoundRobinResolver(resolver *Resolver) *RoundRobinResolver {
	if resolver == nil {
		resolver = NewResolver(Config{})
	}
	return &RoundRobinResolver{resolver: resolver}
}

// Resolver 返回底层 DNS resolver。
func (r *RoundRobinResolver) Resolver() *Resolver {
	if r == nil {
		return nil
	}
	return r.resolver
}

// LookupIP 查询 IP 并按调用次数轮询首选地址。
func (r *RoundRobinResolver) LookupIP(ctx context.Context, host string) ([]net.IP, error) {
	if r == nil || r.resolver == nil {
		return nil, ErrNoAnswer
	}
	ips, err := r.resolver.LookupIP(ctx, host)
	if err != nil || len(ips) <= 1 {
		return ips, err
	}
	start := int(r.next.Add(1)-1) % len(ips)
	return rotateIPs(ips, start), nil
}

// LookupHost 查询字符串形式 IP，并按 LookupIP 的轮询顺序返回。
func (r *RoundRobinResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	ips, err := r.LookupIP(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	return out, nil
}

// RoundRobinResolverGroup 按 key 复用 RoundRobinResolver。
type RoundRobinResolverGroup[K comparable] struct {
	base *ResolverGroup[K]
	mu   sync.RWMutex
	rr   map[K]*RoundRobinResolver
}

// NewRoundRobinResolverGroup 创建按 key 隔离轮询状态的 resolver group。
func NewRoundRobinResolverGroup[K comparable](cfg Config) *RoundRobinResolverGroup[K] {
	return &RoundRobinResolverGroup[K]{
		base: NewResolverGroup[K](cfg),
		rr:   make(map[K]*RoundRobinResolver, 8),
	}
}

// Resolver 返回 key 对应的 round-robin resolver。
func (g *RoundRobinResolverGroup[K]) Resolver(key K) *RoundRobinResolver {
	if g == nil {
		return NewRoundRobinResolver(NewResolver(Config{}))
	}
	g.mu.RLock()
	resolver := g.rr[key]
	g.mu.RUnlock()
	if resolver != nil {
		return resolver
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if resolver = g.rr[key]; resolver != nil {
		return resolver
	}
	resolver = NewRoundRobinResolver(g.base.Resolver(key))
	g.rr[key] = resolver
	return resolver
}

// Delete 删除 key 关联的 round-robin resolver。
func (g *RoundRobinResolverGroup[K]) Delete(key K) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	_, ok := g.rr[key]
	delete(g.rr, key)
	g.mu.Unlock()
	g.base.Delete(key)
	return ok
}

// Clear 清空全部 resolver。
func (g *RoundRobinResolverGroup[K]) Clear() {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.rr = make(map[K]*RoundRobinResolver, 8)
	g.mu.Unlock()
	g.base.Clear()
}

// Size 返回当前缓存的 round-robin resolver 数量。
func (g *RoundRobinResolverGroup[K]) Size() int {
	if g == nil {
		return 0
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.rr)
}

func rotateIPs(ips []net.IP, start int) []net.IP {
	out := make([]net.IP, 0, len(ips))
	out = append(out, ips[start:]...)
	out = append(out, ips[:start]...)
	return out
}

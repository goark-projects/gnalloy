package npcap

import (
	"context"
	"fmt"

	"goark.dev/gnalloy/transport/l2"
)

// Backend 是真实 Npcap 驱动需要实现的最小打开接口。
type Backend interface {
	// OpenNpcap 使用规范化配置打开一个 Npcap Endpoint。
	OpenNpcap(ctx context.Context, cfg Config) (l2.Endpoint, error)
}

// BackendFunc 允许普通函数作为 Npcap Backend 使用。
type BackendFunc func(ctx context.Context, cfg Config) (l2.Endpoint, error)

// OpenNpcap 实现 Backend。
func (f BackendFunc) OpenNpcap(ctx context.Context, cfg Config) (l2.Endpoint, error) {
	return f(ctx, cfg)
}

// Driver 把独立 Npcap 后端适配成 transport/l2.Driver。
type Driver struct {
	Backend Backend
	Config  Config
}

// NewDriver 创建 Npcap Driver 适配器。
func NewDriver(backend Backend, cfg Config) Driver {
	return Driver{Backend: backend, Config: cfg}
}

// Kind 返回该边界对应的 L2 driver 类型。
func Kind() l2.DriverKind {
	return l2.DriverKindNpcap
}

// Open 根据 l2.Config 规范化参数后委派给真实 Npcap 后端。
func (d Driver) Open(ctx context.Context, cfg l2.Config) (l2.Endpoint, error) {
	backend := d.Backend
	if backend == nil {
		backend = defaultBackend()
	}
	if backend == nil {
		return nil, fmt.Errorf("%w: %w", l2.ErrUnsupportedDriver, ErrMissingBackend)
	}
	normalized, err := normalizeConfig(d.Config, cfg)
	if err != nil {
		return nil, err
	}
	return backend.OpenNpcap(ctx, normalized)
}

var _ l2.Driver = Driver{}

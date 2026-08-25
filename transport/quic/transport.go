package quic

import (
	"context"

	"goark.dev/gnalloy/bootstrap"
)

// Transport 是 QUIC 协议引擎的 ServerBootstrap 入口。
// 当前只固定公开契约，完整协议状态机在后续迭代实现。
type Transport struct {
	cfg Config
}

func NewTransport(cfg Config) *Transport {
	return &Transport{cfg: cfg}
}

func (t *Transport) Bind(ctx context.Context, cfg bootstrap.ServerConfig) (bootstrap.Server, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.WorkerGroup == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if cfg.ChildInitializer == nil {
		return nil, bootstrap.ErrMissingChildHandler
	}
	if _, err := NormalizeConfig(t.cfg); err != nil {
		return nil, err
	}
	return nil, ErrNotImplemented
}

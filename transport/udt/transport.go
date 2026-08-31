package udt

import (
	"context"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
)

// Transport 是 UDT 的 Bootstrap/Dialer 适配器。
type Transport struct {
	cfg Config
}

func NewTransport(cfg Config) *Transport {
	return &Transport{cfg: normalizeConfig(cfg)}
}

func (t *Transport) Bind(ctx context.Context, cfg bootstrap.ServerConfig) (bootstrap.Server, error) {
	if cfg.BossGroup == nil || cfg.WorkerGroup == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if cfg.ChildInitializer == nil {
		return nil, bootstrap.ErrMissingChildHandler
	}
	if t == nil || t.cfg.Driver == nil {
		return nil, ErrUnsupportedUDT
	}
	return t.cfg.Driver.Bind(ctx, cfg, t.cfg)
}

func (t *Transport) Dial(ctx context.Context, cfg bootstrap.ClientConfig) (channel.Channel, error) {
	if cfg.Group == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if cfg.Initializer == nil {
		cfg.Initializer = func(channel.Channel) error { return nil }
	}
	if t == nil || t.cfg.Driver == nil {
		return nil, ErrUnsupportedUDT
	}
	return t.cfg.Driver.Dial(ctx, cfg, t.cfg)
}

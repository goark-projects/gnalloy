package rxtx

import (
	"context"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
)

// Transport 是 RXTX/serial client channel 的 Dialer 适配器。
type Transport struct {
	cfg Config
}

func NewTransport(cfg Config) *Transport {
	return &Transport{cfg: cfg}
}

func (t *Transport) Dial(ctx context.Context, cfg bootstrap.ClientConfig) (channel.Channel, error) {
	if cfg.Group == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if cfg.Initializer == nil {
		cfg.Initializer = func(channel.Channel) error { return nil }
	}
	if t == nil || t.cfg.Driver == nil {
		return nil, ErrUnsupportedRXTX
	}
	serialCfg := normalizeConfig(t.cfg, cfg.Address)
	return t.cfg.Driver.Dial(ctx, cfg, serialCfg)
}

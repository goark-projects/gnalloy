package driver

import (
	"context"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
)

// BindUDT 实现 Backend。
func (f BackendFuncs) BindUDT(ctx context.Context, cfg BindConfig) (bootstrap.Server, error) {
	if f.Bind == nil {
		return nil, unsupported(ErrMissingBind)
	}
	return f.Bind(ctx, cfg)
}

// DialUDT 实现 Backend。
func (f BackendFuncs) DialUDT(ctx context.Context, cfg DialConfig) (channel.Channel, error) {
	if f.Dial == nil {
		return nil, unsupported(ErrMissingDial)
	}
	return f.Dial(ctx, cfg)
}

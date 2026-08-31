package driver

import (
	"context"

	"goark.dev/gnalloy/channel"
)

// DialRXTX 实现 Backend。
func (f BackendFunc) DialRXTX(ctx context.Context, cfg DialConfig) (channel.Channel, error) {
	if f == nil {
		return nil, unsupported(ErrMissingDial)
	}
	return f(ctx, cfg)
}

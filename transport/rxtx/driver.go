package rxtx

import (
	"context"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
)

// Driver 把外部串口实现适配到 gnalloy Dialer。
type Driver interface {
	Dial(ctx context.Context, cfg bootstrap.ClientConfig, serialCfg Config) (channel.Channel, error)
}

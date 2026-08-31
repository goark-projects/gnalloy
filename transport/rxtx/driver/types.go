package driver

import (
	"context"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/rxtx"
)

// DialConfig 是真实串口后端执行 Dial 时收到的只读装配快照。
type DialConfig struct {
	Bootstrap bootstrap.ClientConfig
	Serial    rxtx.Config
}

// Backend 是第三方串口库需要实现的最小适配边界。
type Backend interface {
	// DialRXTX 打开串口客户端并返回 gnalloy Channel。
	DialRXTX(ctx context.Context, cfg DialConfig) (channel.Channel, error)
}

// BackendFunc 允许用普通函数装配串口后端，适合小型驱动或测试。
type BackendFunc func(ctx context.Context, cfg DialConfig) (channel.Channel, error)

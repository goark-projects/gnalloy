package driver

import (
	"context"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/udt"
)

// BindConfig 是真实 UDT 后端执行 Bind 时收到的只读装配快照。
type BindConfig struct {
	Bootstrap bootstrap.ServerConfig
	UDT       udt.Config
}

// DialConfig 是真实 UDT 后端执行 Dial 时收到的只读装配快照。
type DialConfig struct {
	Bootstrap bootstrap.ClientConfig
	UDT       udt.Config
}

// Backend 是第三方 UDT 库需要实现的最小适配边界。
type Backend interface {
	// BindUDT 绑定 UDT 服务端并返回 gnalloy Server 句柄。
	BindUDT(ctx context.Context, cfg BindConfig) (bootstrap.Server, error)
	// DialUDT 建立 UDT 客户端连接并返回 gnalloy Channel。
	DialUDT(ctx context.Context, cfg DialConfig) (channel.Channel, error)
}

// BackendFuncs 允许用普通函数装配 UDT 后端，适合小型驱动或测试。
type BackendFuncs struct {
	Bind func(ctx context.Context, cfg BindConfig) (bootstrap.Server, error)
	Dial func(ctx context.Context, cfg DialConfig) (channel.Channel, error)
}

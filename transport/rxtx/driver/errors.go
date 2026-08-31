package driver

import "errors"

var (
	// ErrMissingBackend 表示没有配置串口后端实现。
	ErrMissingBackend = errors.New("gnalloy/transport/rxtx/driver: missing backend")
	// ErrMissingDial 表示函数式后端缺少客户端 Dial 实现。
	ErrMissingDial = errors.New("gnalloy/transport/rxtx/driver: missing dial function")
)

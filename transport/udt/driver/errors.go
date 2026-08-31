package driver

import "errors"

var (
	// ErrMissingBackend 表示没有配置 UDT 后端实现。
	ErrMissingBackend = errors.New("gnalloy/transport/udt/driver: missing backend")
	// ErrMissingBind 表示函数式后端缺少服务端 Bind 实现。
	ErrMissingBind = errors.New("gnalloy/transport/udt/driver: missing bind function")
	// ErrMissingDial 表示函数式后端缺少客户端 Dial 实现。
	ErrMissingDial = errors.New("gnalloy/transport/udt/driver: missing dial function")
)

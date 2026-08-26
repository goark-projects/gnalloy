package l2

import "errors"

var (
	// ErrInvalidConfig 表示 L2 transport 配置缺少必要字段。
	ErrInvalidConfig = errors.New("gnalloy/transport/l2: invalid config")
	// ErrUnsupportedDriver 表示当前平台或配置没有可用二层驱动。
	ErrUnsupportedDriver = errors.New("gnalloy/transport/l2: unsupported driver")
	// ErrInvalidFrame 表示出站或入站二层帧无效。
	ErrInvalidFrame = errors.New("gnalloy/transport/l2: invalid frame")
	// ErrClosed 表示 L2 endpoint 已关闭。
	ErrClosed = errors.New("gnalloy/transport/l2: closed")
)

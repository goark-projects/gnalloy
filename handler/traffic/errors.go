package traffic

import "errors"

var (
	// ErrInvalidConfig 表示 traffic shaping 配置非法。
	ErrInvalidConfig = errors.New("gnalloy/handler/traffic: invalid config")
	// ErrMissingController 表示 Handler 缺少流量整形控制器。
	ErrMissingController = errors.New("gnalloy/handler/traffic: missing controller")
	// ErrClosedHandler 表示 Handler 已经关闭，不能再接受延迟写入。
	ErrClosedHandler = errors.New("gnalloy/handler/traffic: handler closed")
)

package cors

import "errors"

var (
	// ErrInvalidConfig 表示 CORS 配置缺少允许的来源或方法。
	ErrInvalidConfig = errors.New("gnalloy/handler/cors: invalid config")
	// ErrForbiddenOrigin 表示请求 Origin 或预检方法未被允许。
	ErrForbiddenOrigin = errors.New("gnalloy/handler/cors: forbidden origin")
)

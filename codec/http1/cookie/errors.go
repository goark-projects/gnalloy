package cookie

import "errors"

var (
	// ErrInvalidCookie 表示 Cookie 名称、值或属性不满足 HTTP 线协议约束。
	ErrInvalidCookie = errors.New("gnalloy/codec/http1/cookie: invalid cookie")
	// ErrInvalidSameSite 表示 SameSite 属性值不受支持。
	ErrInvalidSameSite = errors.New("gnalloy/codec/http1/cookie: invalid samesite")
)

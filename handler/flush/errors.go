package flush

import "errors"

var (
	// ErrInvalidFlushThreshold 表示显式 flush 阈值非法。
	ErrInvalidFlushThreshold = errors.New("gnalloy/handler/flush: invalid flush threshold")
	// ErrClosedHandler 表示 Handler 已关闭，不能再接收新的 flush。
	ErrClosedHandler = errors.New("gnalloy/handler/flush: handler closed")
)

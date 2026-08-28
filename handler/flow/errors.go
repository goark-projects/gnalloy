package flow

import "errors"

var (
	// ErrInvalidConfig 表示 flow handler 配置非法。
	ErrInvalidConfig = errors.New("gnalloy/handler/flow: invalid config")
	// ErrMissingContext 表示 Handler 尚未加入 Pipeline，不能恢复排队消息。
	ErrMissingContext = errors.New("gnalloy/handler/flow: missing handler context")
	// ErrQueueFull 表示暂停期间的入站消息队列已达到保护上限。
	ErrQueueFull = errors.New("gnalloy/handler/flow: pending inbound queue full")
	// ErrClosedHandler 表示 Handler 已关闭，不能再接收或恢复消息。
	ErrClosedHandler = errors.New("gnalloy/handler/flow: handler closed")
)

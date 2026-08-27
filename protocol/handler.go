package protocol

// Handler 处理统一应用协议请求。
type Handler interface {
	// ServeProtocol 处理请求，并通过 Responder 写回响应。
	ServeProtocol(req Request, responder Responder) error
}

// HandlerFunc 允许普通函数作为 Handler 使用。
type HandlerFunc func(req Request, responder Responder) error

// ServeProtocol 实现 Handler。
func (f HandlerFunc) ServeProtocol(req Request, responder Responder) error {
	if f == nil {
		return ErrInvalidConfig
	}
	return f(req, responder)
}

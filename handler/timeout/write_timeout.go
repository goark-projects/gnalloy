package timeout

import "goark.dev/gnalloy/channel"

// WriteTimeoutHandler 在写空闲超时后抛出异常并关闭 Channel。
type WriteTimeoutHandler struct {
	idle *IdleStateHandler
}

func NewWriteTimeoutHandler(timeoutMillis int64) *WriteTimeoutHandler {
	idle := NewIdleStateHandler(0, timeoutMillis, 0)
	idle.onIdle = func(ctx *channel.HandlerContext, event IdleStateEvent) bool {
		if event.State != WriterIdle {
			return false
		}
		ctx.FireExceptionCaught(ErrWriteTimeout)
		_ = ctx.Close()
		return true
	}
	return &WriteTimeoutHandler{idle: idle}
}

func (h *WriteTimeoutHandler) ChannelActive(ctx *channel.HandlerContext) {
	h.idle.ChannelActive(ctx)
}

func (h *WriteTimeoutHandler) Write(ctx *channel.HandlerContext, msg any) error {
	return h.idle.Write(ctx, msg)
}

func (h *WriteTimeoutHandler) ChannelInactive(ctx *channel.HandlerContext) {
	h.idle.ChannelInactive(ctx)
}

package timeout

import "goark.dev/gnalloy/channel"

// ReadTimeoutHandler 在读空闲超时后抛出异常并关闭 Channel。
type ReadTimeoutHandler struct {
	idle *IdleStateHandler
}

func NewReadTimeoutHandler(timeoutMillis int64) *ReadTimeoutHandler {
	idle := NewIdleStateHandler(timeoutMillis, 0, 0)
	idle.onIdle = func(ctx *channel.HandlerContext, event IdleStateEvent) bool {
		if event.State != ReaderIdle {
			return false
		}
		ctx.FireExceptionCaught(ErrReadTimeout)
		_ = ctx.Close()
		return true
	}
	return &ReadTimeoutHandler{idle: idle}
}

func (h *ReadTimeoutHandler) ChannelActive(ctx *channel.HandlerContext) {
	h.idle.ChannelActive(ctx)
}

func (h *ReadTimeoutHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	h.idle.ChannelRead(ctx, msg)
}

func (h *ReadTimeoutHandler) ChannelInactive(ctx *channel.HandlerContext) {
	h.idle.ChannelInactive(ctx)
}

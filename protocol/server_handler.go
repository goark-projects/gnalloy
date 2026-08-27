package protocol

import "goark.dev/gnalloy/channel"

// NewServerHandler 创建可安装到 Pipeline 的应用协议服务端 handler。
func NewServerHandler(adapter ServerAdapter, handler Handler) channel.Handler {
	return &serverHandler{adapter: adapter, handler: handler}
}

type serverHandler struct {
	adapter ServerAdapter
	handler Handler
}

func (h *serverHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if h == nil || h.adapter == nil || h.handler == nil {
		ctx.FireExceptionCaught(ErrInvalidConfig)
		releaseMessage(msg)
		return
	}
	req, matched, err := h.adapter.ExtractRequest(ctx.Channel(), msg)
	if err != nil {
		releaseMessage(msg)
		ctx.FireExceptionCaught(err)
		return
	}
	if !matched {
		ctx.FireChannelRead(msg)
		return
	}
	defer releaseMessage(msg)
	if err := h.handler.ServeProtocol(req, newResponder(ctx, h.adapter, req)); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

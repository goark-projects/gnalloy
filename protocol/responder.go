package protocol

import "goark.dev/gnalloy/channel"

// Responder 是应用协议服务端的统一响应写回接口。
type Responder interface {
	// Respond 将应用响应负载转换为对应传输消息并立即 flush。
	Respond(payload []byte) error
}

type channelResponder struct {
	ctx     *channel.HandlerContext
	adapter ServerAdapter
	request Request
}

func newResponder(ctx *channel.HandlerContext, adapter ServerAdapter, req Request) Responder {
	return channelResponder{ctx: ctx, adapter: adapter, request: req}
}

// Respond 实现 Responder。
func (r channelResponder) Respond(payload []byte) error {
	if r.ctx == nil || r.adapter == nil {
		return ErrInvalidConfig
	}
	msg, err := r.adapter.BuildResponse(r.ctx.Channel(), r.request, payload)
	if err != nil {
		return err
	}
	if err := r.ctx.Write(msg); err != nil {
		releaseMessage(msg)
		return err
	}
	return r.ctx.Flush()
}

package executor

import (
	"fmt"
	"sync"

	"goark.dev/gnalloy/channel"
)

// InboundHandler 把被代理 Handler 的入站事件提交到业务执行器。
type InboundHandler struct {
	executor Executor
	delegate channel.Handler

	mu    sync.Mutex
	bound Executor
}

// NewInboundHandler 创建入站 offload Handler。
func NewInboundHandler(executor Executor, delegate channel.Handler) *InboundHandler {
	return &InboundHandler{executor: executor, delegate: delegate}
}

// HandlerAdded 校验依赖，并同步执行被代理 Handler 的添加回调。
func (h *InboundHandler) HandlerAdded(ctx *channel.HandlerContext) error {
	if h.delegate == nil {
		return ErrMissingHandler
	}
	if h.executorFor() == nil {
		return ErrMissingExecutor
	}
	if added, ok := h.delegate.(channel.HandlerAddedHandler); ok {
		return added.HandlerAdded(ctx)
	}
	return nil
}

// HandlerRemoved 同步执行被代理 Handler 的移除回调。
func (h *InboundHandler) HandlerRemoved(ctx *channel.HandlerContext) error {
	if removed, ok := h.delegate.(channel.HandlerRemovedHandler); ok {
		return removed.HandlerRemoved(ctx)
	}
	return nil
}

// ChannelRegistered 异步派发注册事件。
func (h *InboundHandler) ChannelRegistered(ctx *channel.HandlerContext) {
	if next, ok := h.delegate.(channel.ChannelRegisteredHandler); ok {
		h.submit(ctx, nil, func() { next.ChannelRegistered(ctx) })
		return
	}
	ctx.FireChannelRegistered()
}

// ChannelUnregistered 异步派发注销事件。
func (h *InboundHandler) ChannelUnregistered(ctx *channel.HandlerContext) {
	if next, ok := h.delegate.(channel.ChannelUnregisteredHandler); ok {
		h.submit(ctx, nil, func() { next.ChannelUnregistered(ctx) })
		return
	}
	ctx.FireChannelUnregistered()
}

// ChannelActive 异步派发激活事件。
func (h *InboundHandler) ChannelActive(ctx *channel.HandlerContext) {
	if next, ok := h.delegate.(channel.ChannelActiveHandler); ok {
		h.submit(ctx, nil, func() { next.ChannelActive(ctx) })
		return
	}
	ctx.FireChannelActive()
}

// ChannelRead 异步派发读事件，提交失败时释放消息并上报异常。
func (h *InboundHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if next, ok := h.delegate.(channel.ChannelReadHandler); ok {
		h.submit(ctx, msg, func() { next.ChannelRead(ctx, msg) })
		return
	}
	ctx.FireChannelRead(msg)
}

// ChannelReadComplete 异步派发读完成事件。
func (h *InboundHandler) ChannelReadComplete(ctx *channel.HandlerContext) {
	if next, ok := h.delegate.(channel.ChannelReadCompleteHandler); ok {
		h.submit(ctx, nil, func() { next.ChannelReadComplete(ctx) })
		return
	}
	ctx.FireChannelReadComplete()
}

// ChannelInactive 异步派发失活事件。
func (h *InboundHandler) ChannelInactive(ctx *channel.HandlerContext) {
	if next, ok := h.delegate.(channel.ChannelInactiveHandler); ok {
		h.submit(ctx, nil, func() { next.ChannelInactive(ctx) })
		return
	}
	ctx.FireChannelInactive()
}

// ChannelWritabilityChanged 异步派发可写状态变化事件。
func (h *InboundHandler) ChannelWritabilityChanged(ctx *channel.HandlerContext) {
	if next, ok := h.delegate.(channel.ChannelWritabilityChangedHandler); ok {
		h.submit(ctx, nil, func() { next.ChannelWritabilityChanged(ctx) })
		return
	}
	ctx.FireChannelWritabilityChanged()
}

// UserEventTriggered 异步派发用户事件。
func (h *InboundHandler) UserEventTriggered(ctx *channel.HandlerContext, event any) {
	if next, ok := h.delegate.(channel.UserEventTriggeredHandler); ok {
		h.submit(ctx, nil, func() { next.UserEventTriggered(ctx, event) })
		return
	}
	ctx.FireUserEventTriggered(event)
}

// FlushComplete 异步派发刷写完成事件。
func (h *InboundHandler) FlushComplete(ctx *channel.HandlerContext) {
	if next, ok := h.delegate.(channel.FlushCompleteHandler); ok {
		h.submit(ctx, nil, func() { next.FlushComplete(ctx) })
		return
	}
	ctx.FireFlushComplete()
}

// ExceptionCaught 异步派发异常事件。
func (h *InboundHandler) ExceptionCaught(ctx *channel.HandlerContext, err error) {
	if next, ok := h.delegate.(channel.ExceptionCaughtHandler); ok {
		h.submit(ctx, nil, func() { next.ExceptionCaught(ctx, err) })
		return
	}
	ctx.FireExceptionCaught(err)
}

func (h *InboundHandler) submit(ctx *channel.HandlerContext, msg any, task Task) {
	executor := h.executorFor()
	if executor == nil {
		releaseMessage(msg)
		ctx.FireExceptionCaught(ErrMissingExecutor)
		return
	}
	if err := executor.Submit(func() {
		defer h.recover(ctx)
		task()
	}); err != nil {
		releaseMessage(msg)
		ctx.FireExceptionCaught(err)
	}
}

func (h *InboundHandler) executorFor() Executor {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.bound != nil {
		return h.bound
	}
	if h.executor == nil {
		return nil
	}
	if chooser, ok := h.executor.(Chooser); ok {
		h.bound = chooser.Next()
	} else {
		h.bound = h.executor
	}
	return h.bound
}

func (h *InboundHandler) recover(ctx *channel.HandlerContext) {
	if v := recover(); v != nil {
		ctx.FireExceptionCaught(fmt.Errorf("%w: %v", ErrHandlerPanic, v))
	}
}

func releaseMessage(msg any) {
	if msg == nil {
		return
	}
	if releasable, ok := msg.(interface{ Release() }); ok {
		releasable.Release()
	}
}

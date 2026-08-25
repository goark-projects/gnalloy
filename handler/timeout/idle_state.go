package timeout

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/timer"
)

type IdleState uint8

const (
	ReaderIdle IdleState = iota
	WriterIdle
	AllIdle
)

type IdleStateEvent struct {
	State IdleState
	First bool
}

// IdleStateHandler 对齐 Netty IdleStateHandler，使用 Channel 绑定的时间轮检测读、写、全空闲。
type IdleStateHandler struct {
	readerIdleMillis int64
	writerIdleMillis int64
	allIdleMillis    int64

	readerTask *timer.Task
	writerTask *timer.Task
	allTask    *timer.Task

	firstReader bool
	firstWriter bool
	firstAll    bool
	onIdle      func(ctx *channel.HandlerContext, event IdleStateEvent) bool
}

func NewIdleStateHandler(readerIdleMillis int64, writerIdleMillis int64, allIdleMillis int64) *IdleStateHandler {
	return &IdleStateHandler{
		readerIdleMillis: readerIdleMillis,
		writerIdleMillis: writerIdleMillis,
		allIdleMillis:    allIdleMillis,
		firstReader:      true,
		firstWriter:      true,
		firstAll:         true,
	}
}

func (h *IdleStateHandler) ChannelActive(ctx *channel.HandlerContext) {
	h.initialize(ctx)
	ctx.FireChannelActive()
}

func (h *IdleStateHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	h.rescheduleReader(ctx)
	h.rescheduleAll(ctx)
	ctx.FireChannelRead(msg)
}

func (h *IdleStateHandler) Write(ctx *channel.HandlerContext, msg any) error {
	h.rescheduleWriter(ctx)
	h.rescheduleAll(ctx)
	return ctx.Write(msg)
}

func (h *IdleStateHandler) ChannelInactive(ctx *channel.HandlerContext) {
	h.destroy(ctx)
	ctx.FireChannelInactive()
}

func (h *IdleStateHandler) initialize(ctx *channel.HandlerContext) {
	h.rescheduleReader(ctx)
	h.rescheduleWriter(ctx)
	h.rescheduleAll(ctx)
}

func (h *IdleStateHandler) destroy(ctx *channel.HandlerContext) {
	h.cancel(ctx, &h.readerTask)
	h.cancel(ctx, &h.writerTask)
	h.cancel(ctx, &h.allTask)
}

func (h *IdleStateHandler) rescheduleReader(ctx *channel.HandlerContext) {
	if h.readerIdleMillis <= 0 {
		return
	}
	h.cancel(ctx, &h.readerTask)
	h.readerTask = h.schedule(ctx, h.readerIdleMillis, func() {
		first := h.firstReader
		h.firstReader = false
		h.fireIdle(ctx, IdleStateEvent{State: ReaderIdle, First: first})
		h.readerTask = h.schedule(ctx, h.readerIdleMillis, func() {
			h.fireReaderIdle(ctx)
		})
	})
}

func (h *IdleStateHandler) rescheduleWriter(ctx *channel.HandlerContext) {
	if h.writerIdleMillis <= 0 {
		return
	}
	h.cancel(ctx, &h.writerTask)
	h.writerTask = h.schedule(ctx, h.writerIdleMillis, func() {
		first := h.firstWriter
		h.firstWriter = false
		h.fireIdle(ctx, IdleStateEvent{State: WriterIdle, First: first})
		h.writerTask = h.schedule(ctx, h.writerIdleMillis, func() {
			h.fireWriterIdle(ctx)
		})
	})
}

func (h *IdleStateHandler) rescheduleAll(ctx *channel.HandlerContext) {
	if h.allIdleMillis <= 0 {
		return
	}
	h.cancel(ctx, &h.allTask)
	h.allTask = h.schedule(ctx, h.allIdleMillis, func() {
		first := h.firstAll
		h.firstAll = false
		h.fireIdle(ctx, IdleStateEvent{State: AllIdle, First: first})
		h.allTask = h.schedule(ctx, h.allIdleMillis, func() {
			h.fireAllIdle(ctx)
		})
	})
}

func (h *IdleStateHandler) fireReaderIdle(ctx *channel.HandlerContext) {
	h.fireIdle(ctx, IdleStateEvent{State: ReaderIdle})
	h.readerTask = h.schedule(ctx, h.readerIdleMillis, func() {
		h.fireReaderIdle(ctx)
	})
}

func (h *IdleStateHandler) fireWriterIdle(ctx *channel.HandlerContext) {
	h.fireIdle(ctx, IdleStateEvent{State: WriterIdle})
	h.writerTask = h.schedule(ctx, h.writerIdleMillis, func() {
		h.fireWriterIdle(ctx)
	})
}

func (h *IdleStateHandler) fireAllIdle(ctx *channel.HandlerContext) {
	h.fireIdle(ctx, IdleStateEvent{State: AllIdle})
	h.allTask = h.schedule(ctx, h.allIdleMillis, func() {
		h.fireAllIdle(ctx)
	})
}

func (h *IdleStateHandler) fireIdle(ctx *channel.HandlerContext, event IdleStateEvent) {
	if h.onIdle != nil && h.onIdle(ctx, event) {
		return
	}
	ctx.FireUserEventTriggered(event)
}

func (h *IdleStateHandler) schedule(ctx *channel.HandlerContext, delayMillis int64, fn func()) *timer.Task {
	task, err := ctx.Channel().ScheduleTimer(delayMillis, timer.CallbackFunc(func(timer.Context, *timer.Task) {
		fn()
	}))
	if err != nil {
		ctx.FireExceptionCaught(err)
		return nil
	}
	return task
}

func (h *IdleStateHandler) cancel(ctx *channel.HandlerContext, task **timer.Task) {
	if *task == nil {
		return
	}
	ctx.Channel().CancelTimer(*task)
	*task = nil
}

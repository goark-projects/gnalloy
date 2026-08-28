package traffic

import (
	"sync"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/internal/message"
	"goark.dev/gnalloy/timer"
)

// Handler 是单 Channel 的 traffic shaping 处理器。
type Handler struct {
	controller *Controller
	stats      counters

	mu                sync.Mutex
	writes            []pendingWrite
	flushWaiters      []channel.Promise
	flushRequested    bool
	writeTimerRunning bool
	closed            bool
	pendingWriteBytes int64
}

type pendingWrite struct {
	msg       any
	promise   channel.Promise
	dueMillis int64
	bytes     int
}

// NewChannelHandler 创建每个 Handler 独享限速器的流量整形处理器。
func NewChannelHandler(cfg Config) (*Handler, error) {
	controller, err := NewController(cfg)
	if err != nil {
		return nil, err
	}
	return NewHandler(controller), nil
}

// NewHandler 使用指定 Controller 创建处理器。多个 Handler 共享同一 Controller 即为全局限速。
func NewHandler(controller *Controller) *Handler {
	return &Handler{controller: controller}
}

// HandlerAdded 校验 traffic shaping 依赖。
func (h *Handler) HandlerAdded(*channel.HandlerContext) error {
	if h.controller == nil {
		return ErrMissingController
	}
	return nil
}

// HandlerRemoved 释放尚未下发的出站消息。
func (h *Handler) HandlerRemoved(*channel.HandlerContext) error {
	h.closePending(ErrClosedHandler)
	return nil
}

// ChannelInactive 关闭排队写入并继续传播失活事件。
func (h *Handler) ChannelInactive(ctx *channel.HandlerContext) {
	h.closePending(ErrClosedHandler)
	ctx.FireChannelInactive()
}

// ChannelRead 根据读限速决定立即传播或延迟传播入站消息。
func (h *Handler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	bytes := MessageSize(msg)
	h.stats.readBytes.Add(int64(nonNegative(bytes)))
	delay := h.controller.reserveRead(bytes)
	if delay <= 0 {
		ctx.FireChannelRead(msg)
		return
	}
	h.stats.delayedReads.Add(1)
	if _, err := ctx.Channel().ScheduleTimer(delay, timer.CallbackFunc(func(timer.Context, *timer.Task) {
		if h.isClosed() {
			releaseMessage(msg)
			return
		}
		ctx.FireChannelRead(msg)
	})); err != nil {
		releaseMessage(msg)
		ctx.FireExceptionCaught(err)
	}
}

// Write 根据写限速决定立即下发或排队延迟写入。
func (h *Handler) Write(ctx *channel.HandlerContext, msg any) error {
	return h.write(ctx, msg, nil)
}

// WriteFuture 根据写限速返回可观察完成状态的异步写入 Future。
func (h *Handler) WriteFuture(ctx *channel.HandlerContext, msg any) channel.Future {
	promise := channel.NewPromise()
	if err := h.write(ctx, msg, promise); err != nil {
		promise.SetFailure(err)
	}
	return promise
}

// Flush 在存在延迟写队列时推迟到底层写入排空后执行。
func (h *Handler) Flush(ctx *channel.HandlerContext) error {
	if h.markFlush(nil) {
		return nil
	}
	return ctx.Flush()
}

// FlushFuture 返回可观察完成状态的异步 flush Future。
func (h *Handler) FlushFuture(ctx *channel.HandlerContext) channel.Future {
	promise := channel.NewPromise()
	if h.markFlush(promise) {
		return promise
	}
	return ctx.FlushFuture()
}

// Stats 返回该 Handler 的局部统计快照。
func (h *Handler) Stats() Snapshot {
	if h == nil {
		return Snapshot{}
	}
	h.mu.Lock()
	pendingWrites := int64(len(h.writes))
	pendingWriteBytes := h.pendingWriteBytes
	h.mu.Unlock()
	return h.stats.snapshot(pendingWrites, pendingWriteBytes)
}

func (h *Handler) write(ctx *channel.HandlerContext, msg any, promise channel.Promise) error {
	if h.controller == nil {
		releaseMessage(msg)
		return ErrMissingController
	}
	bytes := MessageSize(msg)
	h.stats.writtenBytes.Add(int64(nonNegative(bytes)))
	delay := h.controller.reserveWrite(bytes)
	if delay <= 0 && !h.hasPendingWrites() {
		return h.writeDownstream(ctx, pendingWrite{msg: msg, promise: promise, bytes: bytes})
	}
	h.stats.delayedWrites.Add(1)
	return h.enqueueWrite(ctx, pendingWrite{
		msg:       msg,
		promise:   promise,
		dueMillis: h.controller.nowMillis() + delay,
		bytes:     bytes,
	})
}

func (h *Handler) enqueueWrite(ctx *channel.HandlerContext, write pendingWrite) error {
	var scheduleDelay int64
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		releaseMessage(write.msg)
		return ErrClosedHandler
	}
	h.writes = append(h.writes, write)
	h.pendingWriteBytes += int64(nonNegative(write.bytes))
	if !h.writeTimerRunning {
		h.writeTimerRunning = true
		scheduleDelay = delayUntil(h.controller.nowMillis(), h.writes[0].dueMillis)
	}
	h.mu.Unlock()

	if scheduleDelay >= 0 {
		if err := h.scheduleWriteDrain(ctx, scheduleDelay); err != nil {
			h.closePending(err)
			return err
		}
	}
	return nil
}

func (h *Handler) markFlush(promise channel.Promise) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.writes) == 0 {
		return false
	}
	h.flushRequested = true
	if promise != nil {
		h.flushWaiters = append(h.flushWaiters, promise)
	}
	return true
}

func (h *Handler) drainWrites(ctx *channel.HandlerContext) {
	var (
		ready     []pendingWrite
		waiters   []channel.Promise
		flush     bool
		nextDelay int64 = -1
	)
	now := h.controller.nowMillis()
	h.mu.Lock()
	for len(h.writes) > 0 && h.writes[0].dueMillis <= now {
		write := h.writes[0]
		copy(h.writes, h.writes[1:])
		h.writes[len(h.writes)-1] = pendingWrite{}
		h.writes = h.writes[:len(h.writes)-1]
		h.pendingWriteBytes -= int64(nonNegative(write.bytes))
		ready = append(ready, write)
	}
	if len(h.writes) == 0 {
		h.writeTimerRunning = false
		flush = h.flushRequested
		h.flushRequested = false
		waiters = h.flushWaiters
		h.flushWaiters = nil
	} else {
		nextDelay = delayUntil(now, h.writes[0].dueMillis)
	}
	h.mu.Unlock()

	for _, write := range ready {
		if err := h.writeDownstream(ctx, write); err != nil {
			ctx.FireExceptionCaught(err)
		}
	}
	if flush {
		h.flushDownstream(ctx, waiters)
	}
	if nextDelay >= 0 {
		if err := h.scheduleWriteDrain(ctx, nextDelay); err != nil {
			h.closePending(err)
			ctx.FireExceptionCaught(err)
		}
	}
}

func (h *Handler) writeDownstream(ctx *channel.HandlerContext, write pendingWrite) error {
	if write.promise == nil {
		return ctx.Write(write.msg)
	}
	future := ctx.WriteFuture(write.msg)
	future.AddListener(func(f channel.Future) {
		if err := f.Err(); err != nil {
			write.promise.SetFailure(err)
			return
		}
		write.promise.SetSuccess()
	})
	return nil
}

func (h *Handler) flushDownstream(ctx *channel.HandlerContext, waiters []channel.Promise) {
	future := ctx.FlushFuture()
	future.AddListener(func(f channel.Future) {
		err := f.Err()
		if err != nil {
			ctx.FireExceptionCaught(err)
		}
		for _, waiter := range waiters {
			if err != nil {
				waiter.SetFailure(err)
			} else {
				waiter.SetSuccess()
			}
		}
	})
}

func (h *Handler) scheduleWriteDrain(ctx *channel.HandlerContext, delayMillis int64) error {
	_, err := ctx.Channel().ScheduleTimer(delayMillis, timer.CallbackFunc(func(timer.Context, *timer.Task) {
		if h.isClosed() {
			return
		}
		h.drainWrites(ctx)
	}))
	return err
}

func (h *Handler) hasPendingWrites() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.writes) > 0
}

func (h *Handler) isClosed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

func (h *Handler) closePending(err error) {
	var (
		writes  []pendingWrite
		waiters []channel.Promise
	)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	writes = h.writes
	waiters = h.flushWaiters
	h.writes = nil
	h.flushWaiters = nil
	h.flushRequested = false
	h.pendingWriteBytes = 0
	h.mu.Unlock()

	for _, write := range writes {
		releaseMessage(write.msg)
		if write.promise != nil {
			write.promise.SetFailure(err)
		}
	}
	for _, waiter := range waiters {
		waiter.SetFailure(err)
	}
}

func delayUntil(nowMillis int64, dueMillis int64) int64 {
	if dueMillis <= nowMillis {
		return 0
	}
	return dueMillis - nowMillis
}

// MessageSize 返回 traffic shaping 使用的消息字节数。
func MessageSize(msg any) int {
	switch v := msg.(type) {
	case nil:
		return 0
	case buffer.ByteBuf:
		return v.ReadableBytes()
	case []byte:
		return len(v)
	case string:
		return len(v)
	case interface{ ReadableBytes() int }:
		return v.ReadableBytes()
	default:
		return 0
	}
}

func releaseMessage(msg any) {
	message.Release(msg)
}

package flush

import (
	"errors"
	"sync"
	"sync/atomic"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/timer"
)

// ConsolidationHandler 合并连续 flush，降低高频小写入场景下的系统调用次数。
type ConsolidationHandler struct {
	explicitFlushAfterFlushes       int
	consolidateWhenNoReadInProgress bool
	noReadFlushDelayMillis          int64

	mu             sync.Mutex
	readInProgress bool
	pendingFlushes int
	scheduled      bool
	waiters        []channel.Promise
	closed         bool

	downstreamFlushes   atomic.Uint64
	consolidatedFlushes atomic.Uint64
}

// NewConsolidationHandler 创建 flush 聚合处理器。
func NewConsolidationHandler(config Config) (*ConsolidationHandler, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	return &ConsolidationHandler{
		explicitFlushAfterFlushes:       normalized.ExplicitFlushAfterFlushes,
		consolidateWhenNoReadInProgress: normalized.ConsolidateWhenNoReadInProgress,
		noReadFlushDelayMillis:          normalized.ConsolidateNoReadFlushDelayMillis,
	}, nil
}

// ChannelRead 标记当前处于读循环，并继续传播入站消息。
func (h *ConsolidationHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	h.mu.Lock()
	if !h.closed {
		h.readInProgress = true
	}
	h.mu.Unlock()
	ctx.FireChannelRead(msg)
}

// ChannelReadComplete 在读完成时下发一次累计 flush，并继续传播读完成事件。
func (h *ConsolidationHandler) ChannelReadComplete(ctx *channel.HandlerContext) {
	if err := h.readComplete(ctx); err != nil {
		ctx.FireExceptionCaught(err)
	}
	ctx.FireChannelReadComplete()
}

// ChannelInactive 在失活前尽量下发已聚合的 flush，并继续传播失活事件。
func (h *ConsolidationHandler) ChannelInactive(ctx *channel.HandlerContext) {
	if err := h.flushPending(ctx); err != nil {
		ctx.FireExceptionCaught(err)
	}
	h.markClosed()
	ctx.FireChannelInactive()
}

// HandlerRemoved 在移除前尽量下发已聚合的 flush。
func (h *ConsolidationHandler) HandlerRemoved(ctx *channel.HandlerContext) error {
	err := h.flushPending(ctx)
	h.markClosed()
	return err
}

// Flush 聚合 flush 请求，必要时再下发到后续出站链路。
func (h *ConsolidationHandler) Flush(ctx *channel.HandlerContext) error {
	return h.flush(ctx, nil)
}

// FlushFuture 返回聚合 flush 的异步完成结果。
func (h *ConsolidationHandler) FlushFuture(ctx *channel.HandlerContext) channel.Future {
	promise := channel.NewPromise()
	if err := h.flush(ctx, promise); err != nil {
		promise.SetFailure(err)
	}
	return promise
}

// Close 在关闭前先下发已聚合的 flush。
func (h *ConsolidationHandler) Close(ctx *channel.HandlerContext) error {
	if err := h.flushPending(ctx); err != nil {
		return err
	}
	h.markClosed()
	return ctx.Close()
}

// Stats 返回当前聚合状态快照。
func (h *ConsolidationHandler) Stats() Stats {
	if h == nil {
		return Stats{}
	}
	h.mu.Lock()
	stats := Stats{
		PendingFlushes: h.pendingFlushes,
		ReadInProgress: h.readInProgress,
		Scheduled:      h.scheduled,
	}
	h.mu.Unlock()
	stats.DownstreamFlushes = h.downstreamFlushes.Load()
	stats.ConsolidatedFlushes = h.consolidatedFlushes.Load()
	return stats
}

func (h *ConsolidationHandler) flush(ctx *channel.HandlerContext, promise channel.Promise) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		if promise != nil {
			promise.SetFailure(ErrClosedHandler)
		}
		return ErrClosedHandler
	}
	h.pendingFlushes++
	if promise != nil {
		h.waiters = append(h.waiters, promise)
	}
	thresholdReached := h.pendingFlushes >= h.explicitFlushAfterFlushes
	switch {
	case h.readInProgress:
		if !thresholdReached {
			h.mu.Unlock()
			return nil
		}
		waiters, flushes := h.drainLocked()
		h.mu.Unlock()
		return h.flushDownstream(ctx, waiters, flushes)
	case !h.consolidateWhenNoReadInProgress || thresholdReached:
		waiters, flushes := h.drainLocked()
		h.mu.Unlock()
		return h.flushDownstream(ctx, waiters, flushes)
	case h.scheduled:
		h.mu.Unlock()
		return nil
	default:
		h.scheduled = true
		h.mu.Unlock()
		return h.scheduleNoReadFlush(ctx)
	}
}

func (h *ConsolidationHandler) readComplete(ctx *channel.HandlerContext) error {
	h.mu.Lock()
	h.readInProgress = false
	if h.pendingFlushes == 0 {
		h.mu.Unlock()
		return nil
	}
	waiters, flushes := h.drainLocked()
	h.mu.Unlock()
	return h.flushDownstream(ctx, waiters, flushes)
}

func (h *ConsolidationHandler) flushPending(ctx *channel.HandlerContext) error {
	h.mu.Lock()
	if h.pendingFlushes == 0 {
		h.readInProgress = false
		h.scheduled = false
		h.mu.Unlock()
		return nil
	}
	waiters, flushes := h.drainLocked()
	h.readInProgress = false
	h.mu.Unlock()
	return h.flushDownstream(ctx, waiters, flushes)
}

func (h *ConsolidationHandler) scheduleNoReadFlush(ctx *channel.HandlerContext) error {
	_, err := ctx.Channel().ScheduleTimer(h.noReadFlushDelayMillis, timer.CallbackFunc(func(timer.Context, *timer.Task) {
		if flushErr := h.scheduledFlush(ctx); flushErr != nil {
			ctx.FireExceptionCaught(flushErr)
		}
	}))
	if err == nil {
		return nil
	}
	if errors.Is(err, channel.ErrNoTimer) {
		waiters, flushes := h.cancelScheduleAndDrain()
		return h.flushDownstream(ctx, waiters, flushes)
	}
	waiters := h.failScheduled(err)
	completeWaiters(waiters, err)
	return err
}

func (h *ConsolidationHandler) scheduledFlush(ctx *channel.HandlerContext) error {
	h.mu.Lock()
	h.scheduled = false
	if h.closed || h.readInProgress || h.pendingFlushes == 0 {
		h.mu.Unlock()
		return nil
	}
	waiters, flushes := h.drainLocked()
	h.mu.Unlock()
	return h.flushDownstream(ctx, waiters, flushes)
}

func (h *ConsolidationHandler) cancelScheduleAndDrain() ([]channel.Promise, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.scheduled = false
	if h.pendingFlushes == 0 {
		return nil, 0
	}
	return h.drainLocked()
}

func (h *ConsolidationHandler) failScheduled(err error) []channel.Promise {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.scheduled = false
	waiters := h.waiters
	h.waiters = nil
	h.pendingFlushes = 0
	return waiters
}

func (h *ConsolidationHandler) drainLocked() ([]channel.Promise, int) {
	waiters := h.waiters
	flushes := h.pendingFlushes
	h.waiters = nil
	h.pendingFlushes = 0
	h.scheduled = false
	return waiters, flushes
}

func (h *ConsolidationHandler) flushDownstream(ctx *channel.HandlerContext, waiters []channel.Promise, flushes int) error {
	if flushes <= 0 {
		return nil
	}
	h.recordFlush(flushes)
	if len(waiters) == 0 {
		return ctx.Flush()
	}
	future := ctx.FlushFuture()
	future.AddListener(func(f channel.Future) {
		err := f.Err()
		if err != nil {
			ctx.FireExceptionCaught(err)
		}
		completeWaiters(waiters, err)
	})
	if future.IsDone() {
		return future.Err()
	}
	return nil
}

func (h *ConsolidationHandler) recordFlush(flushes int) {
	h.downstreamFlushes.Add(1)
	if flushes > 1 {
		h.consolidatedFlushes.Add(uint64(flushes - 1))
	}
}

func (h *ConsolidationHandler) markClosed() {
	h.mu.Lock()
	h.closed = true
	h.readInProgress = false
	h.scheduled = false
	h.pendingFlushes = 0
	waiters := h.waiters
	h.waiters = nil
	h.mu.Unlock()
	completeWaiters(waiters, ErrClosedHandler)
}

func completeWaiters(waiters []channel.Promise, err error) {
	for _, waiter := range waiters {
		if waiter == nil {
			continue
		}
		if err != nil {
			waiter.SetFailure(err)
		} else {
			waiter.SetSuccess()
		}
	}
}

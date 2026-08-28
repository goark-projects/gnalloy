package flow

import (
	"sync"
	"sync/atomic"

	"goark.dev/gnalloy/channel"
)

// Handler 在业务 Pipeline 内提供显式入站暂停、排队和恢复语义。
type Handler struct {
	cfg Config

	paused atomic.Bool

	mu                 sync.Mutex
	ctx                *channel.HandlerContext
	pending            []pendingRead
	pendingBytes       int64
	readCompleteQueued bool
	droppedMessages    uint64
	closed             bool
}

type pendingRead struct {
	msg   any
	bytes int
}

// NewHandler 创建入站流控处理器。
func NewHandler(cfg Config) (*Handler, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	h := &Handler{cfg: normalized}
	h.paused.Store(normalized.StartPaused)
	return h, nil
}

// HandlerAdded 绑定当前 Channel，并读取已有 AutoRead 选项作为初始暂停信号。
func (h *Handler) HandlerAdded(ctx *channel.HandlerContext) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return ErrClosedHandler
	}
	h.ctx = ctx
	if h.paused.Load() || !channel.OptionAutoRead.Get(ctx.Channel().Options()) {
		h.paused.Store(true)
		channel.OptionAutoRead.Set(ctx.Channel().Options(), false)
	}
	return nil
}

// HandlerRemoved 释放暂停期间积压的入站消息。
func (h *Handler) HandlerRemoved(*channel.HandlerContext) error {
	h.closePending()
	return nil
}

// ChannelInactive 释放积压消息并继续传播失活事件。
func (h *Handler) ChannelInactive(ctx *channel.HandlerContext) {
	h.closePending()
	ctx.FireChannelInactive()
}

// ChannelRead 在暂停时排队消息，在运行时直接透传。
func (h *Handler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if !h.paused.Load() {
		ctx.FireChannelRead(msg)
		return
	}
	h.enqueue(ctx, msg)
}

// ChannelReadComplete 在暂停期间合并完成事件，恢复时只传播一次。
func (h *Handler) ChannelReadComplete(ctx *channel.HandlerContext) {
	if !h.paused.Load() {
		ctx.FireChannelReadComplete()
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.readCompleteQueued = true
	h.mu.Unlock()
}

// Pause 暂停后续入站消息传播，并同步关闭 Channel AutoRead 选项。
func (h *Handler) Pause() {
	ctx := h.setPaused(true)
	if ctx != nil {
		channel.OptionAutoRead.Set(ctx.Channel().Options(), false)
	}
}

// Resume 恢复入站传播，按原顺序下发积压消息并触发一次手动读。
func (h *Handler) Resume() error {
	ctx, reads, fireComplete, err := h.drainPending()
	if err != nil {
		return err
	}
	channel.OptionAutoRead.Set(ctx.Channel().Options(), true)
	for _, read := range reads {
		ctx.FireChannelRead(read.msg)
	}
	if fireComplete {
		ctx.FireChannelReadComplete()
	}
	return ctx.Channel().Read()
}

// Snapshot 返回当前队列与暂停状态。
func (h *Handler) Snapshot() Snapshot {
	if h == nil {
		return Snapshot{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return Snapshot{
		Paused:          h.paused.Load(),
		PendingMessages: len(h.pending),
		PendingBytes:    h.pendingBytes,
		DroppedMessages: h.droppedMessages,
	}
}

func (h *Handler) enqueue(ctx *channel.HandlerContext, msg any) {
	size := nonNegative(MessageSize(msg))
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		releaseMessage(msg)
		ctx.FireExceptionCaught(ErrClosedHandler)
		return
	}
	if h.queueFull(size) {
		h.droppedMessages++
		h.mu.Unlock()
		releaseMessage(msg)
		ctx.FireExceptionCaught(ErrQueueFull)
		return
	}
	h.pending = append(h.pending, pendingRead{msg: msg, bytes: size})
	h.pendingBytes += int64(size)
	h.mu.Unlock()
}

func (h *Handler) queueFull(size int) bool {
	if len(h.pending) >= h.cfg.MaxPendingMessages {
		return true
	}
	return h.pendingBytes+int64(size) > h.cfg.MaxPendingBytes
}

func (h *Handler) setPaused(paused bool) *channel.HandlerContext {
	h.mu.Lock()
	ctx := h.ctx
	if !h.closed {
		h.paused.Store(paused)
	}
	h.mu.Unlock()
	return ctx
}

func (h *Handler) drainPending() (*channel.HandlerContext, []pendingRead, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, nil, false, ErrClosedHandler
	}
	if h.ctx == nil {
		return nil, nil, false, ErrMissingContext
	}
	reads := h.pending
	fireComplete := h.readCompleteQueued
	h.pending = nil
	h.pendingBytes = 0
	h.readCompleteQueued = false
	h.paused.Store(false)
	return h.ctx, reads, fireComplete, nil
}

func (h *Handler) closePending() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	h.paused.Store(true)
	reads := h.pending
	h.pending = nil
	h.pendingBytes = 0
	h.readCompleteQueued = false
	h.mu.Unlock()

	for _, read := range reads {
		releaseMessage(read.msg)
	}
}

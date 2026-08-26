package logging

import (
	"context"
	"fmt"
	"log/slog"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

const logMessage = "gnalloy channel event"

// MessageSizer 估算日志中记录的消息大小。
type MessageSizer interface {
	MessageSize(msg any) int64
}

// MessageSizerFunc 把函数适配为 MessageSizer。
type MessageSizerFunc func(msg any) int64

// MessageSize 返回函数计算出的消息大小。
func (f MessageSizerFunc) MessageSize(msg any) int64 {
	if f == nil {
		return 0
	}
	return f(msg)
}

// Config 定义 LoggingHandler 的日志依赖和字段策略。
type Config struct {
	// Logger 接收结构化日志；nil 时使用 slog.Default()。
	Logger *slog.Logger
	// Level 是事件日志级别；零值为 slog.LevelInfo。
	Level slog.Level
	// Sizer 估算读写消息字节数；nil 时仅使用 ByteBuf/[]byte/string 快速路径。
	Sizer MessageSizer
}

// Handler 记录 Channel 生命周期、入站、出站和异常事件，并继续传播事件。
type Handler struct {
	logger *slog.Logger
	level  slog.Level
	sizer  MessageSizer
}

// NewHandler 创建一个 Pipeline 日志处理器。
func NewHandler(config Config) *Handler {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		logger: logger,
		level:  config.Level,
		sizer:  config.Sizer,
	}
}

// ChannelRegistered 记录注册事件并继续传播。
func (h *Handler) ChannelRegistered(ctx *channel.HandlerContext) {
	h.log(ctx, "channel_registered")
	ctx.FireChannelRegistered()
}

// ChannelUnregistered 记录注销事件并继续传播。
func (h *Handler) ChannelUnregistered(ctx *channel.HandlerContext) {
	h.log(ctx, "channel_unregistered")
	ctx.FireChannelUnregistered()
}

// ChannelActive 记录激活事件并继续传播。
func (h *Handler) ChannelActive(ctx *channel.HandlerContext) {
	h.log(ctx, "channel_active")
	ctx.FireChannelActive()
}

// ChannelInactive 记录失活事件并继续传播。
func (h *Handler) ChannelInactive(ctx *channel.HandlerContext) {
	h.log(ctx, "channel_inactive")
	ctx.FireChannelInactive()
}

// ChannelRead 记录入站消息事件并继续传播；消息所有权保持不变。
func (h *Handler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	h.log(ctx, "channel_read", h.messageAttrs(msg)...)
	ctx.FireChannelRead(msg)
}

// ChannelReadComplete 记录读完成事件并继续传播。
func (h *Handler) ChannelReadComplete(ctx *channel.HandlerContext) {
	h.log(ctx, "channel_read_complete")
	ctx.FireChannelReadComplete()
}

// ChannelWritabilityChanged 记录可写状态变化并继续传播。
func (h *Handler) ChannelWritabilityChanged(ctx *channel.HandlerContext) {
	h.log(ctx, "channel_writability_changed")
	ctx.FireChannelWritabilityChanged()
}

// UserEventTriggered 记录用户事件并继续传播。
func (h *Handler) UserEventTriggered(ctx *channel.HandlerContext, event any) {
	h.log(ctx, "user_event", slog.String("event_type", typeName(event)))
	ctx.FireUserEventTriggered(event)
}

// ExceptionCaught 记录异常事件并继续传播。
func (h *Handler) ExceptionCaught(ctx *channel.HandlerContext, err error) {
	h.log(ctx, "exception", slog.Any("error", err))
	ctx.FireExceptionCaught(err)
}

// Write 记录出站写入并继续向下游写入；消息所有权由下游接管。
func (h *Handler) Write(ctx *channel.HandlerContext, msg any) error {
	h.log(ctx, "write", h.messageAttrs(msg)...)
	return ctx.Write(msg)
}

// Flush 记录 flush 事件并继续向下游 flush。
func (h *Handler) Flush(ctx *channel.HandlerContext) error {
	h.log(ctx, "flush")
	return ctx.Flush()
}

// FlushComplete 记录 flush 完成事件并继续传播。
func (h *Handler) FlushComplete(ctx *channel.HandlerContext) {
	h.log(ctx, "flush_complete")
	ctx.FireFlushComplete()
}

// Close 记录 close 事件并继续向下游关闭。
func (h *Handler) Close(ctx *channel.HandlerContext) error {
	h.log(ctx, "close")
	return ctx.Close()
}

func (h *Handler) log(ctx *channel.HandlerContext, event string, attrs ...slog.Attr) {
	if h == nil || h.logger == nil {
		return
	}
	logCtx := context.Background()
	if !h.logger.Enabled(logCtx, h.level) {
		return
	}
	all := make([]slog.Attr, 0, 4+len(attrs))
	all = append(all, slog.String("event", event))
	if ctx != nil {
		all = append(all, slog.String("handler", ctx.Name()))
		if ch := ctx.Channel(); ch != nil {
			all = append(all, slog.Uint64("channel_id", uint64(ch.ID())))
		}
	}
	all = append(all, attrs...)
	h.logger.LogAttrs(logCtx, h.level, logMessage, all...)
}

func (h *Handler) messageAttrs(msg any) []slog.Attr {
	attrs := []slog.Attr{slog.String("message_type", typeName(msg))}
	if size := h.messageSize(msg); size >= 0 {
		attrs = append(attrs, slog.Int64("message_size", size))
	}
	return attrs
}

func (h *Handler) messageSize(msg any) int64 {
	if h != nil && h.sizer != nil {
		size := h.sizer.MessageSize(msg)
		if size >= 0 {
			return size
		}
		return -1
	}
	switch v := msg.(type) {
	case nil:
		return 0
	case buffer.ByteBuf:
		return int64(v.ReadableBytes())
	case []byte:
		return int64(len(v))
	case string:
		return int64(len(v))
	case interface{ ReadableBytes() int }:
		size := v.ReadableBytes()
		if size < 0 {
			return -1
		}
		return int64(size)
	default:
		return -1
	}
}

func typeName(value any) string {
	if value == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T", value)
}

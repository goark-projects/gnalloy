package metrics

import (
	"time"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/observability"
	"goark.dev/gnalloy/transport"
)

// Config 定义 ChannelMetricsHandler 的依赖。
type Config struct {
	// Recorder 接收聚合事件；nil 时使用空实现。
	Recorder observability.ChannelRecorder
	// Sizer 估算读写消息字节数；nil 时使用 ReadableBytesSizer。
	Sizer observability.MessageSizer
}

// ChannelMetricsHandler 把 Pipeline 生命周期、读写、flush、close 和异常事件记录为指标。
type ChannelMetricsHandler struct {
	recorder observability.ChannelRecorder
	latency  observability.ChannelLatencyRecorder
	sizer    observability.MessageSizer
}

func NewChannelMetricsHandler(config Config) *ChannelMetricsHandler {
	recorder := observability.NormalizeChannelRecorder(config.Recorder)
	latency, _ := recorder.(observability.ChannelLatencyRecorder)
	return &ChannelMetricsHandler{
		recorder: recorder,
		latency:  latency,
		sizer:    observability.NormalizeMessageSizer(config.Sizer),
	}
}

func (h *ChannelMetricsHandler) ChannelRegistered(ctx *channel.HandlerContext) {
	h.recorder.RecordChannelRegistered(channelID(ctx))
	ctx.FireChannelRegistered()
}

func (h *ChannelMetricsHandler) ChannelUnregistered(ctx *channel.HandlerContext) {
	h.recorder.RecordChannelUnregistered(channelID(ctx))
	ctx.FireChannelUnregistered()
}

func (h *ChannelMetricsHandler) ChannelActive(ctx *channel.HandlerContext) {
	h.recorder.RecordChannelActive(channelID(ctx))
	ctx.FireChannelActive()
}

func (h *ChannelMetricsHandler) ChannelInactive(ctx *channel.HandlerContext) {
	h.recorder.RecordChannelInactive(channelID(ctx))
	ctx.FireChannelInactive()
}

func (h *ChannelMetricsHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	id := channelID(ctx)
	h.recorder.RecordChannelRead(id, h.messageSize(msg))
	if h.latency == nil {
		ctx.FireChannelRead(msg)
		return
	}
	start := time.Now()
	ctx.FireChannelRead(msg)
	h.latency.RecordChannelReadDuration(id, time.Since(start))
}

func (h *ChannelMetricsHandler) ChannelReadComplete(ctx *channel.HandlerContext) {
	h.recorder.RecordChannelReadComplete(channelID(ctx))
	ctx.FireChannelReadComplete()
}

func (h *ChannelMetricsHandler) ExceptionCaught(ctx *channel.HandlerContext, err error) {
	h.recorder.RecordException(channelID(ctx), err)
	ctx.FireExceptionCaught(err)
}

func (h *ChannelMetricsHandler) Write(ctx *channel.HandlerContext, msg any) error {
	id := channelID(ctx)
	h.recorder.RecordChannelWrite(id, h.messageSize(msg))
	var start time.Time
	if h.latency != nil {
		start = time.Now()
	}
	err := ctx.Write(msg)
	if h.latency != nil {
		h.latency.RecordChannelWriteDuration(id, time.Since(start))
	}
	if err != nil {
		h.recorder.RecordException(id, err)
		return err
	}
	return nil
}

func (h *ChannelMetricsHandler) Flush(ctx *channel.HandlerContext) error {
	id := channelID(ctx)
	h.recorder.RecordChannelFlush(id)
	var start time.Time
	if h.latency != nil {
		start = time.Now()
	}
	err := ctx.Flush()
	if h.latency != nil {
		h.latency.RecordChannelFlushDuration(id, time.Since(start))
	}
	if err != nil {
		h.recorder.RecordException(id, err)
		return err
	}
	return nil
}

func (h *ChannelMetricsHandler) Close(ctx *channel.HandlerContext) error {
	id := channelID(ctx)
	h.recorder.RecordChannelClose(id)
	var start time.Time
	if h.latency != nil {
		start = time.Now()
	}
	err := ctx.Close()
	if h.latency != nil {
		h.latency.RecordChannelCloseDuration(id, time.Since(start))
	}
	if err != nil {
		h.recorder.RecordException(id, err)
		return err
	}
	return nil
}

func (h *ChannelMetricsHandler) messageSize(msg any) int64 {
	n := h.sizer.MessageSize(msg)
	if n < 0 {
		return 0
	}
	return n
}

func channelID(ctx *channel.HandlerContext) transport.ChannelID {
	if ctx == nil || ctx.Channel() == nil {
		return 0
	}
	return ctx.Channel().ID()
}

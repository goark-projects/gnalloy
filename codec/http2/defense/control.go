package defense

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/http2"
)

// ControlFrameLimitConfig 描述出站控制帧队列防护。
type ControlFrameLimitConfig struct {
	// MaxQueuedFrames 是 flush 前允许暂存的控制帧数，0 表示不限制。
	MaxQueuedFrames int
}

// ControlFrameLimitEncoder 限制 flush 前出站控制帧数量。
type ControlFrameLimitEncoder struct {
	cfg     ControlFrameLimitConfig
	pending int
}

// NewControlFrameLimitEncoder 创建控制帧出站防护 handler。
func NewControlFrameLimitEncoder(cfg ControlFrameLimitConfig) *ControlFrameLimitEncoder {
	return &ControlFrameLimitEncoder{cfg: cfg}
}

// PendingControlFrames 返回当前 flush 边界内已通过的控制帧数量。
func (e *ControlFrameLimitEncoder) PendingControlFrames() int {
	if e == nil {
		return 0
	}
	return e.pending
}

// Write 在控制帧超过预算时拒绝继续入队。
func (e *ControlFrameLimitEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	if e == nil || e.cfg.MaxQueuedFrames <= 0 || !isControlFrame(msg) {
		return ctx.Write(msg)
	}
	if e.pending >= e.cfg.MaxQueuedFrames {
		releaseTypedFrame(msg)
		return ErrTooManyControlFrames
	}
	if err := ctx.Write(msg); err != nil {
		return err
	}
	e.pending++
	return nil
}

// Flush 成功越过 flush 边界后清理控制帧预算。
func (e *ControlFrameLimitEncoder) Flush(ctx *channel.HandlerContext) error {
	if err := ctx.Flush(); err != nil {
		return err
	}
	e.pending = 0
	return nil
}

// ChannelInactive 在连接关闭时清理计数。
func (e *ControlFrameLimitEncoder) ChannelInactive(ctx *channel.HandlerContext) {
	e.pending = 0
	ctx.FireChannelInactive()
}

func isControlFrame(msg any) bool {
	switch msg.(type) {
	case http2.RSTStreamFrame, http2.SettingsFrame, http2.PingFrame, http2.GoAwayFrame, http2.WindowUpdateFrame, http2.PriorityFrame:
		return true
	default:
		return false
	}
}

func releaseTypedFrame(msg any) {
	if frame, ok := msg.(interface{ Release() }); ok {
		frame.Release()
	}
}

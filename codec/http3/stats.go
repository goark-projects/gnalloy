package http3

import (
	"sync/atomic"

	"goark.dev/gnalloy/channel"
)

// FrameDirection 描述 HTTP/3 frame 在 Pipeline 中的方向。
type FrameDirection uint8

const (
	FrameDirectionInbound FrameDirection = iota + 1
	FrameDirectionOutbound
)

// StatsSnapshot 是 HTTP/3 frame 统计的无锁快照。
type StatsSnapshot struct {
	InboundFrames       uint64
	OutboundFrames      uint64
	InboundBytes        uint64
	OutboundBytes       uint64
	InboundDataBytes    uint64
	OutboundDataBytes   uint64
	InboundHeaderBytes  uint64
	OutboundHeaderBytes uint64
	SettingsFrames      uint64
	GoAwayFrames        uint64
	UnknownFrames       uint64
}

// StatsRecorder 是 HTTP/3 frame 统计接入点。
type StatsRecorder interface {
	RecordHTTP3Frame(direction FrameDirection, frameType FrameType, payloadBytes int)
}

// AtomicStatsRecorder 是低基数、并发安全的 HTTP/3 frame 统计器。
type AtomicStatsRecorder struct {
	inboundFrames       atomic.Uint64
	outboundFrames      atomic.Uint64
	inboundBytes        atomic.Uint64
	outboundBytes       atomic.Uint64
	inboundDataBytes    atomic.Uint64
	outboundDataBytes   atomic.Uint64
	inboundHeaderBytes  atomic.Uint64
	outboundHeaderBytes atomic.Uint64
	settingsFrames      atomic.Uint64
	goAwayFrames        atomic.Uint64
	unknownFrames       atomic.Uint64
}

// NewAtomicStatsRecorder 创建 HTTP/3 原子统计器。
func NewAtomicStatsRecorder() *AtomicStatsRecorder {
	return &AtomicStatsRecorder{}
}

// RecordHTTP3Frame 记录单个 HTTP/3 frame，不持有消息引用。
func (r *AtomicStatsRecorder) RecordHTTP3Frame(direction FrameDirection, frameType FrameType, payloadBytes int) {
	if r == nil {
		return
	}
	if payloadBytes < 0 {
		payloadBytes = 0
	}
	switch direction {
	case FrameDirectionInbound:
		r.inboundFrames.Add(1)
		r.inboundBytes.Add(uint64(payloadBytes))
		if frameType == FrameData {
			r.inboundDataBytes.Add(uint64(payloadBytes))
		}
		if frameType == FrameHeaders {
			r.inboundHeaderBytes.Add(uint64(payloadBytes))
		}
	case FrameDirectionOutbound:
		r.outboundFrames.Add(1)
		r.outboundBytes.Add(uint64(payloadBytes))
		if frameType == FrameData {
			r.outboundDataBytes.Add(uint64(payloadBytes))
		}
		if frameType == FrameHeaders {
			r.outboundHeaderBytes.Add(uint64(payloadBytes))
		}
	default:
		return
	}
	switch frameType {
	case FrameSettings:
		r.settingsFrames.Add(1)
	case FrameGoAway:
		r.goAwayFrames.Add(1)
	default:
		if frameType != FrameData &&
			frameType != FrameHeaders &&
			frameType != FrameCancelPush &&
			frameType != FramePushPromise &&
			frameType != FrameMaxPushID &&
			frameType != FramePriorityUpdateStream &&
			frameType != FramePriorityUpdatePush {
			r.unknownFrames.Add(1)
		}
	}
}

// Snapshot 返回当前统计快照。
func (r *AtomicStatsRecorder) Snapshot() StatsSnapshot {
	if r == nil {
		return StatsSnapshot{}
	}
	return StatsSnapshot{
		InboundFrames:       r.inboundFrames.Load(),
		OutboundFrames:      r.outboundFrames.Load(),
		InboundBytes:        r.inboundBytes.Load(),
		OutboundBytes:       r.outboundBytes.Load(),
		InboundDataBytes:    r.inboundDataBytes.Load(),
		OutboundDataBytes:   r.outboundDataBytes.Load(),
		InboundHeaderBytes:  r.inboundHeaderBytes.Load(),
		OutboundHeaderBytes: r.outboundHeaderBytes.Load(),
		SettingsFrames:      r.settingsFrames.Load(),
		GoAwayFrames:        r.goAwayFrames.Load(),
		UnknownFrames:       r.unknownFrames.Load(),
	}
}

// StatsHandler 在不接管消息所有权的前提下记录 HTTP/3 frame 统计。
type StatsHandler struct {
	recorder StatsRecorder
}

// NewStatsHandler 创建 HTTP/3 frame 统计 handler。
func NewStatsHandler(recorder StatsRecorder) *StatsHandler {
	return &StatsHandler{recorder: recorder}
}

// ChannelRead 记录入站 frame 后继续传播。
func (h *StatsHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if frameType, payloadBytes, ok := frameMetric(msg); ok && h.recorder != nil {
		h.recorder.RecordHTTP3Frame(FrameDirectionInbound, frameType, payloadBytes)
	}
	ctx.FireChannelRead(msg)
}

// Write 记录出站 frame 后继续传播。
func (h *StatsHandler) Write(ctx *channel.HandlerContext, msg any) error {
	if frameType, payloadBytes, ok := frameMetric(msg); ok && h.recorder != nil {
		h.recorder.RecordHTTP3Frame(FrameDirectionOutbound, frameType, payloadBytes)
	}
	return ctx.Write(msg)
}

func frameMetric(msg any) (FrameType, int, bool) {
	switch frame := msg.(type) {
	case DataFrame:
		return FrameData, readable(frame.Data), true
	case *DataFrame:
		if frame == nil {
			return 0, 0, false
		}
		return FrameData, readable(frame.Data), true
	case HeadersFrame:
		return FrameHeaders, readable(frame.HeaderBlock), true
	case *HeadersFrame:
		if frame == nil {
			return 0, 0, false
		}
		return FrameHeaders, readable(frame.HeaderBlock), true
	case CancelPushFrame:
		return FrameCancelPush, 0, true
	case SettingsFrame:
		return FrameSettings, len(frame.Settings) * 2, true
	case PushPromiseFrame:
		return FramePushPromise, readable(frame.HeaderBlock), true
	case *PushPromiseFrame:
		if frame == nil {
			return 0, 0, false
		}
		return FramePushPromise, readable(frame.HeaderBlock), true
	case GoAwayFrame:
		return FrameGoAway, 0, true
	case MaxPushIDFrame:
		return FrameMaxPushID, 0, true
	case PriorityUpdateFrame:
		return frame.Type, readable(frame.FieldValue), true
	case *PriorityUpdateFrame:
		if frame == nil {
			return 0, 0, false
		}
		return frame.Type, readable(frame.FieldValue), true
	case UnknownFrame:
		return frame.Type, readable(frame.Payload), true
	case *UnknownFrame:
		if frame == nil {
			return 0, 0, false
		}
		return frame.Type, readable(frame.Payload), true
	default:
		return 0, 0, false
	}
}

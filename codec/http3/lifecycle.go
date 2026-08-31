package http3

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

// LifecycleEventType 描述 HTTP/3 连接级语义帧事件类型。
type LifecycleEventType uint8

const (
	LifecycleEventSettings LifecycleEventType = iota + 1
	LifecycleEventGoAway
	LifecycleEventMaxPushID
	LifecycleEventCancelPush
	LifecycleEventPriorityUpdate
)

// LifecycleEvent 是 HTTP/3 control/request stream 上可观测的连接级事件。
type LifecycleEvent struct {
	Type               LifecycleEventType
	Settings           []Setting
	ID                 uint64
	PushID             uint64
	PriorityFrameType  FrameType
	PriorityElementID  uint64
	PriorityFieldValue []byte
}

// LifecycleHandler 把 HTTP/3 连接级语义帧转换为 user event，并保留原帧继续传播。
type LifecycleHandler struct{}

// NewLifecycleHandler 创建 HTTP/3 生命周期事件 handler。
func NewLifecycleHandler() *LifecycleHandler {
	return &LifecycleHandler{}
}

// ChannelRead 发布轻量事件；payload 所有权仍由原始 frame 持有。
func (h *LifecycleHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if event, ok := lifecycleEvent(msg); ok {
		ctx.FireUserEventTriggered(event)
	}
	ctx.FireChannelRead(msg)
}

func lifecycleEvent(msg any) (LifecycleEvent, bool) {
	switch frame := msg.(type) {
	case SettingsFrame:
		return LifecycleEvent{Type: LifecycleEventSettings, Settings: cloneSettings(frame.Settings)}, true
	case GoAwayFrame:
		return LifecycleEvent{Type: LifecycleEventGoAway, ID: frame.ID}, true
	case MaxPushIDFrame:
		return LifecycleEvent{Type: LifecycleEventMaxPushID, PushID: frame.PushID}, true
	case CancelPushFrame:
		return LifecycleEvent{Type: LifecycleEventCancelPush, PushID: frame.PushID}, true
	case PriorityUpdateFrame:
		return LifecycleEvent{
			Type:               LifecycleEventPriorityUpdate,
			PriorityFrameType:  frame.Type,
			PriorityElementID:  frame.ElementID,
			PriorityFieldValue: cloneByteBuf(frame.FieldValue),
		}, true
	default:
		return LifecycleEvent{}, false
	}
}

func cloneByteBuf(src buffer.ByteBuf) []byte {
	if src == nil || src.ReadableBytes() == 0 {
		return nil
	}
	out := make([]byte, src.ReadableBytes())
	if buffer.CopyReadableBytes(out, src) != len(out) {
		return nil
	}
	return out
}

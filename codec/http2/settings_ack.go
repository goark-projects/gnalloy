package http2

import "goark.dev/gnalloy/channel"

// SettingsReceivedEvent 表示对端 SETTINGS 帧已接收并已排队 ACK。
type SettingsReceivedEvent struct {
	Settings []Setting
}

// SettingsAckHandler 为非 ACK SETTINGS 帧写出 SETTINGS ACK，并保留原帧继续向后传播。
type SettingsAckHandler struct{}

// NewSettingsAckHandler 创建 HTTP/2 SETTINGS ACK 生命周期处理器。
func NewSettingsAckHandler() *SettingsAckHandler {
	return &SettingsAckHandler{}
}

// ChannelRead 对齐 RFC 7540：普通 SETTINGS 必须确认，SETTINGS ACK 不能再次确认。
func (h *SettingsAckHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	settings, ok := msg.(SettingsFrame)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if settings.Ack {
		ctx.FireChannelRead(msg)
		return
	}
	if err := ctx.WriteAndFlush(SettingsFrame{Ack: true}); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireUserEventTriggered(SettingsReceivedEvent{Settings: cloneSettings(settings.Settings)})
	ctx.FireChannelRead(msg)
}

func cloneSettings(settings []Setting) []Setting {
	if len(settings) == 0 {
		return nil
	}
	out := make([]Setting, len(settings))
	copy(out, settings)
	return out
}

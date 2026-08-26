package http3

import "goark.dev/gnalloy/channel"

// ControlStreamHandler 校验 HTTP/3 control stream 的 frame 顺序和非法帧。
type ControlStreamHandler struct {
	seenType     bool
	seenSettings bool
}

// NewControlStreamHandler 创建 HTTP/3 control stream 校验 handler。
func NewControlStreamHandler() *ControlStreamHandler {
	return &ControlStreamHandler{}
}

// ChannelRead 校验 SETTINGS 必须是 control stream 首帧，且禁止 DATA/HEADERS 等请求流帧。
func (h *ControlStreamHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch frame := msg.(type) {
	case StreamTypeFrame:
		if h.seenType || frame.Type != StreamTypeControl {
			ctx.FireExceptionCaught(ErrUnsupportedFrame)
			return
		}
		h.seenType = true
		ctx.FireChannelRead(frame)
	case SettingsFrame:
		if h.seenSettings {
			ctx.FireExceptionCaught(ErrInvalidFrameOrder)
			return
		}
		if err := validateSettings(frame.Settings); err != nil {
			ctx.FireExceptionCaught(err)
			return
		}
		h.seenSettings = true
		ctx.FireChannelRead(frame)
	case DataFrame:
		frame.Release()
		h.rejectDataFrame(ctx)
	case HeadersFrame:
		frame.Release()
		h.rejectDataFrame(ctx)
	case PushPromiseFrame:
		frame.Release()
		h.rejectDataFrame(ctx)
	default:
		if !h.seenSettings {
			releaseMessage(msg)
			ctx.FireExceptionCaught(ErrInvalidFrameOrder)
			return
		}
		ctx.FireChannelRead(msg)
	}
}

func (h *ControlStreamHandler) rejectDataFrame(ctx *channel.HandlerContext) {
	if !h.seenSettings {
		ctx.FireExceptionCaught(ErrInvalidFrameOrder)
		return
	}
	ctx.FireExceptionCaught(ErrUnsupportedFrame)
}

func validateSettings(settings []Setting) error {
	if len(settings) < 2 {
		return nil
	}
	seen := make(map[uint64]struct{}, min(len(settings), 8))
	for _, setting := range settings {
		if _, ok := seen[setting.ID]; ok {
			return ErrDuplicateSetting
		}
		seen[setting.ID] = struct{}{}
	}
	return nil
}

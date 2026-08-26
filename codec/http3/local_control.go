package http3

import "goark.dev/gnalloy/channel"

// LocalControlStreamHandler 在本端 control stream 激活时发送 SETTINGS。
type LocalControlStreamHandler struct {
	settings []Setting
	wrote    bool
}

// NewLocalControlStreamHandler 创建本端 control stream SETTINGS 写入器。
func NewLocalControlStreamHandler(settings []Setting) (*LocalControlStreamHandler, error) {
	if err := validateSettings(settings); err != nil {
		return nil, err
	}
	return &LocalControlStreamHandler{settings: cloneSettings(settings)}, nil
}

// ChannelActive 先写出 SETTINGS 并刷新，再把 active 事件交给后续 handler。
func (h *LocalControlStreamHandler) ChannelActive(ctx *channel.HandlerContext) {
	if h.wrote {
		ctx.FireChannelActive()
		return
	}
	h.wrote = true
	if err := ctx.Write(SettingsFrame{Settings: h.settings}); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Flush(); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelActive()
}

func cloneSettings(settings []Setting) []Setting {
	if len(settings) == 0 {
		return nil
	}
	cloned := make([]Setting, len(settings))
	copy(cloned, settings)
	return cloned
}

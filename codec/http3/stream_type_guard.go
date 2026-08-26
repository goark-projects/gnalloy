package http3

import "goark.dev/gnalloy/channel"

// StreamTypeGuard 校验单向 QUIC stream 的 HTTP/3 stream type。
type StreamTypeGuard struct {
	expected StreamType
	seen     bool
}

// NewStreamTypeGuard 创建固定 stream type 的入站校验器。
func NewStreamTypeGuard(expected StreamType) *StreamTypeGuard {
	return &StreamTypeGuard{expected: expected}
}

// ChannelRead 要求首个语义消息是匹配的 StreamTypeFrame，后续消息保持透传。
func (g *StreamTypeGuard) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch frame := msg.(type) {
	case StreamTypeFrame:
		if g.seen || frame.Type != g.expected {
			ctx.FireExceptionCaught(ErrUnsupportedFrame)
			return
		}
		g.seen = true
		ctx.FireChannelRead(frame)
	default:
		if !g.seen {
			releaseMessage(msg)
			ctx.FireExceptionCaught(ErrInvalidFrameOrder)
			return
		}
		ctx.FireChannelRead(msg)
	}
}

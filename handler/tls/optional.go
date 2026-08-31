package tls

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

// OptionalEvent 描述 OptionalHandler 对连接起始字节的协议判定。
type OptionalEvent struct {
	TLS         bool
	ClientHello ClientHello
}

// OptionalHandler 在明文和 TLS 共享端口上探测 ClientHello。
type OptionalHandler struct {
	cumulation *buffer.CompositeByteBuf
	decided    bool
}

// NewOptionalHandler 创建 Optional TLS 探测处理器。
func NewOptionalHandler() *OptionalHandler {
	return &OptionalHandler{}
}

// ChannelRead 按 TLS 记录头做最小探测，判定后进入透明转发。
func (h *OptionalHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if h.decided {
		ctx.FireChannelRead(msg)
		return
	}
	in, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if in.ReadableBytes() == 0 {
		in.Release()
		return
	}
	if h.cumulation == nil && h.detectPlaintextFast(ctx, in) {
		return
	}
	h.append(in)
	h.tryDetect(ctx)
}

// ChannelInactive 释放仍未完成协议判定的半包。
func (h *OptionalHandler) ChannelInactive(ctx *channel.HandlerContext) {
	h.releaseCumulation()
	ctx.FireChannelInactive()
}

// HandlerRemoved 释放仍未完成协议判定的半包。
func (h *OptionalHandler) HandlerRemoved(*channel.HandlerContext) error {
	h.releaseCumulation()
	return nil
}

func (h *OptionalHandler) detectPlaintextFast(ctx *channel.HandlerContext, in buffer.ByteBuf) bool {
	first, ok := in.GetByte(in.ReaderIndex())
	if ok && first != tlsRecordTypeHandshake {
		h.firePlaintext(ctx, in)
		return true
	}
	return false
}

func (h *OptionalHandler) append(in buffer.ByteBuf) {
	if h.cumulation == nil {
		h.cumulation = buffer.NewCompositeByteBuf()
	}
	h.cumulation.Append(in)
}

func (h *OptionalHandler) tryDetect(ctx *channel.HandlerContext) {
	if h.cumulation == nil || h.cumulation.ReadableBytes() == 0 {
		return
	}
	reader := h.cumulation.ReaderIndex()
	first, ok := h.cumulation.GetByte(reader)
	if ok && first != tlsRecordTypeHandshake {
		h.firePlaintext(ctx, h.takeCumulation())
		return
	}
	if h.cumulation.ReadableBytes() < 3 {
		return
	}
	major, _ := h.cumulation.GetByte(reader + 1)
	minor, _ := h.cumulation.GetByte(reader + 2)
	if !isTLSRecordVersion(major, minor) {
		h.firePlaintext(ctx, h.takeCumulation())
		return
	}
	if h.cumulation.ReadableBytes() < tlsRecordHeaderLen {
		return
	}
	recordLen, err := h.cumulation.ReadUnsigned(reader+3, 2, buffer.BigEndian)
	if err != nil || recordLen == 0 || recordLen > maxTLSRecordPayload {
		h.fail(ctx, ErrInvalidClientHello)
		return
	}
	if h.cumulation.ReadableBytes() < tlsRecordHeaderLen+int(recordLen) {
		return
	}
	h.inspectTLS(ctx)
}

func (h *OptionalHandler) inspectTLS(ctx *channel.HandlerContext) {
	raw := make([]byte, h.cumulation.ReadableBytes())
	if buffer.CopyReadableBytes(raw, h.cumulation) != len(raw) {
		h.fail(ctx, ErrInvalidClientHello)
		return
	}
	hello, complete, err := InspectClientHello(raw)
	if err != nil {
		h.fail(ctx, err)
		return
	}
	if !complete {
		return
	}
	h.decided = true
	ctx.FireUserEventTriggered(OptionalEvent{TLS: true, ClientHello: hello})
	ctx.FireUserEventTriggered(StartEvent{})
	ctx.FireChannelRead(h.takeCumulation())
}

func (h *OptionalHandler) firePlaintext(ctx *channel.HandlerContext, msg buffer.ByteBuf) {
	h.decided = true
	ctx.FireUserEventTriggered(OptionalEvent{})
	ctx.FireChannelRead(msg)
}

func (h *OptionalHandler) fail(ctx *channel.HandlerContext, err error) {
	h.decided = true
	h.releaseCumulation()
	ctx.FireExceptionCaught(err)
	_ = ctx.Close()
}

func (h *OptionalHandler) takeCumulation() buffer.ByteBuf {
	out := h.cumulation
	h.cumulation = nil
	return out
}

func (h *OptionalHandler) releaseCumulation() {
	if h.cumulation == nil {
		return
	}
	h.cumulation.Release()
	h.cumulation = nil
}

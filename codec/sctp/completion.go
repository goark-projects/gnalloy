package sctp

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

// MessageCompletionHandler 聚合同一 SCTP stream 上的分片消息。
type MessageCompletionHandler struct {
	pending map[fragmentKey]*buffer.CompositeByteBuf
}

// NewMessageCompletionHandler 创建 SCTP 分片聚合器。
func NewMessageCompletionHandler() *MessageCompletionHandler {
	return &MessageCompletionHandler{}
}

func (h *MessageCompletionHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	m, ok := msg.(Message)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if !m.valid() {
		ctx.FireExceptionCaught(ErrInvalidMessage)
		return
	}
	if !m.Fragmented {
		ctx.FireChannelRead(m)
		return
	}
	if m.Complete {
		h.complete(ctx, m)
		return
	}
	h.appendFragment(m)
}

func (h *MessageCompletionHandler) ChannelInactive(ctx *channel.HandlerContext) {
	h.releasePending()
	ctx.FireChannelInactive()
}

func (h *MessageCompletionHandler) complete(ctx *channel.HandlerContext, m Message) {
	key := keyOf(m)
	pending := h.pending[key]
	if pending == nil {
		ctx.FireExceptionCaught(ErrMissingFragmentStart)
		m.Release()
		return
	}
	delete(h.pending, key)
	pending.Append(m.Payload)
	m.Payload = nil
	ctx.FireChannelRead(Message{
		ProtocolIdentifier: m.ProtocolIdentifier,
		StreamIdentifier:   m.StreamIdentifier,
		Unordered:          m.Unordered,
		Complete:           true,
		Payload:            pending,
	})
}

func (h *MessageCompletionHandler) appendFragment(m Message) {
	if h.pending == nil {
		h.pending = make(map[fragmentKey]*buffer.CompositeByteBuf, 4)
	}
	key := keyOf(m)
	pending := h.pending[key]
	if pending == nil {
		pending = buffer.NewCompositeByteBuf()
		h.pending[key] = pending
	}
	pending.Append(m.Payload)
}

func (h *MessageCompletionHandler) releasePending() {
	for key, pending := range h.pending {
		delete(h.pending, key)
		pending.Release()
	}
}

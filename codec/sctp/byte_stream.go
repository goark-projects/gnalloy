package sctp

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

// InboundByteStreamHandler 将匹配 stream 的 SCTP Message 解包为 ByteBuf。
type InboundByteStreamHandler struct {
	*codec.MessageToMessageDecoder
	protocolIdentifier uint32
	streamIdentifier   uint16
}

// NewInboundByteStreamHandler 创建入站 SCTP byte-stream 解包器。
func NewInboundByteStreamHandler(protocolIdentifier uint32, streamIdentifier uint16) *InboundByteStreamHandler {
	h := &InboundByteStreamHandler{
		protocolIdentifier: protocolIdentifier,
		streamIdentifier:   streamIdentifier,
	}
	h.MessageToMessageDecoder = codec.NewMessageToMessageDecoder(h)
	return h
}

func (h *InboundByteStreamHandler) AcceptInboundMessage(msg any) bool {
	m, ok := msg.(Message)
	return ok && m.ProtocolIdentifier == h.protocolIdentifier && m.StreamIdentifier == h.streamIdentifier
}

func (h *InboundByteStreamHandler) Decode(_ *channel.HandlerContext, msg any, out *codec.MessageList) error {
	m := msg.(Message)
	if !m.valid() {
		return ErrInvalidMessage
	}
	m.Payload.Retain()
	out.Add(m.Payload)
	return nil
}

// OutboundByteStreamHandler 将 ByteBuf 包装为指定 stream 的 SCTP Message。
type OutboundByteStreamHandler struct {
	*codec.MessageToMessageEncoder
	protocolIdentifier uint32
	streamIdentifier   uint16
	unordered          bool
}

// NewOutboundByteStreamHandler 创建有序出站 SCTP byte-stream 包装器。
func NewOutboundByteStreamHandler(protocolIdentifier uint32, streamIdentifier uint16) *OutboundByteStreamHandler {
	return NewOutboundByteStreamHandlerWithOrder(protocolIdentifier, streamIdentifier, false)
}

// NewOutboundByteStreamHandlerWithOrder 创建可配置有序性的出站包装器。
func NewOutboundByteStreamHandlerWithOrder(protocolIdentifier uint32, streamIdentifier uint16, unordered bool) *OutboundByteStreamHandler {
	h := &OutboundByteStreamHandler{
		protocolIdentifier: protocolIdentifier,
		streamIdentifier:   streamIdentifier,
		unordered:          unordered,
	}
	h.MessageToMessageEncoder = codec.NewMessageToMessageEncoder(h)
	return h
}

func (h *OutboundByteStreamHandler) AcceptOutboundMessage(msg any) bool {
	_, ok := msg.(buffer.ByteBuf)
	return ok
}

func (h *OutboundByteStreamHandler) Encode(_ *channel.HandlerContext, msg any, out *codec.MessageList) error {
	payload := msg.(buffer.ByteBuf)
	payload.Retain()
	out.Add(Message{
		ProtocolIdentifier: h.protocolIdentifier,
		StreamIdentifier:   h.streamIdentifier,
		Unordered:          h.unordered,
		Complete:           true,
		Payload:            payload,
	})
	return nil
}

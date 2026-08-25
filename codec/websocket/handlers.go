package websocket

import (
	"encoding/binary"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

type FragmentAggregator struct {
	maxMessageLength int
	opcode           byte
	parts            *buffer.CompositeByteBuf
	readable         int
}

func NewFragmentAggregator(maxMessageLength int) *FragmentAggregator {
	return &FragmentAggregator{maxMessageLength: maxMessageLength}
}

func (h *FragmentAggregator) ChannelRead(ctx *channel.HandlerContext, msg any) {
	frame, ok := msg.(Frame)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if isControlOpcode(frame.Opcode) {
		ctx.FireChannelRead(frame)
		return
	}
	if isDataOpcode(frame.Opcode) {
		h.readDataFrame(ctx, frame)
		return
	}
	if frame.Opcode == OpcodeContinuation {
		h.readContinuation(ctx, frame)
		return
	}
	releaseFrame(frame)
	ctx.FireExceptionCaught(ErrControlFrameInvalid)
}

func (h *FragmentAggregator) ChannelInactive(ctx *channel.HandlerContext) {
	h.releaseParts()
	ctx.FireChannelInactive()
}

func (h *FragmentAggregator) readDataFrame(ctx *channel.HandlerContext, frame Frame) {
	if h.parts != nil {
		releaseFrame(frame)
		ctx.FireExceptionCaught(ErrFragmentInProgress)
		return
	}
	if frame.Final {
		ctx.FireChannelRead(frame)
		return
	}
	h.opcode = frame.Opcode
	h.parts = buffer.NewCompositeByteBuf()
	h.readable = 0
	if !h.appendPayload(frame.Payload) {
		ctx.FireExceptionCaught(codec.ErrFrameTooLong)
		return
	}
}

func (h *FragmentAggregator) readContinuation(ctx *channel.HandlerContext, frame Frame) {
	if h.parts == nil {
		releaseFrame(frame)
		ctx.FireExceptionCaught(ErrUnexpectedContinuation)
		return
	}
	if !h.appendPayload(frame.Payload) {
		ctx.FireExceptionCaught(codec.ErrFrameTooLong)
		return
	}
	if !frame.Final {
		return
	}
	out := Frame{Final: true, Opcode: h.opcode, Payload: h.parts}
	h.parts = nil
	h.opcode = 0
	h.readable = 0
	ctx.FireChannelRead(out)
}

func (h *FragmentAggregator) appendPayload(payload buffer.ByteBuf) bool {
	if payload == nil {
		return true
	}
	next := h.readable + payload.ReadableBytes()
	if h.maxMessageLength > 0 && next > h.maxMessageLength {
		payload.Release()
		h.releaseParts()
		return false
	}
	h.parts.Append(payload)
	h.readable = next
	return true
}

func (h *FragmentAggregator) releaseParts() {
	if h.parts != nil {
		h.parts.Release()
		h.parts = nil
	}
	h.opcode = 0
	h.readable = 0
}

type ControlFrameHandler struct{}

func NewControlFrameHandler() *ControlFrameHandler {
	return &ControlFrameHandler{}
}

func (h *ControlFrameHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	frame, ok := msg.(Frame)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	switch frame.Opcode {
	case OpcodePing:
		if err := ctx.Channel().WriteAndFlush(Frame{Final: true, Opcode: OpcodePong, Payload: frame.Payload}); err != nil {
			releaseFrame(Frame{Payload: frame.Payload})
			ctx.FireExceptionCaught(err)
		}
	case OpcodeClose:
		if err := ctx.Channel().WriteAndFlush(Frame{Final: true, Opcode: OpcodeClose, Payload: frame.Payload}); err != nil {
			releaseFrame(Frame{Payload: frame.Payload})
			ctx.FireExceptionCaught(err)
			return
		}
		_ = ctx.Close()
	default:
		ctx.FireChannelRead(frame)
	}
}

type CloseStatus struct {
	Code   uint16
	Reason string
}

func NewCloseFrame(ctx *channel.HandlerContext, code uint16, reason string) (Frame, error) {
	size := 2 + len(reason)
	payload, err := ctx.Channel().Allocator().Acquire(size)
	if err != nil {
		return Frame{}, err
	}
	var tmp [2]byte
	binary.BigEndian.PutUint16(tmp[:], code)
	if _, err := payload.WriteBytes(tmp[:]); err != nil {
		payload.Release()
		return Frame{}, err
	}
	if _, err := payload.WriteBytes([]byte(reason)); err != nil {
		payload.Release()
		return Frame{}, err
	}
	return Frame{Final: true, Opcode: OpcodeClose, Payload: payload}, nil
}

func ParseCloseStatus(frame Frame) (CloseStatus, bool) {
	if frame.Opcode != OpcodeClose || frame.Payload == nil || frame.Payload.ReadableBytes() < 2 {
		return CloseStatus{}, false
	}
	data := frame.Payload.Bytes()
	return CloseStatus{Code: binary.BigEndian.Uint16(data[:2]), Reason: string(data[2:])}, true
}

func releaseFrame(frame Frame) {
	if frame.Payload != nil {
		frame.Payload.Release()
	}
}

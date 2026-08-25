package websocket

import (
	"encoding/binary"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/handler/timeout"
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

const (
	CloseStatusNormalClosure    uint16 = 1000
	CloseStatusGoingAway        uint16 = 1001
	CloseStatusInvalidFrameData uint16 = 1007
)

// CloseState 描述 WebSocket close 握手的当前状态。
type CloseState uint8

const (
	CloseStateOpen CloseState = iota
	CloseStateCloseSent
	CloseStateCloseReceived
	CloseStateClosed
)

// CloseStateEvent 在 close 握手状态变化时向 Pipeline 传播。
type CloseStateEvent struct {
	State  CloseState
	Status CloseStatus
	Remote bool
}

type ControlFrameHandler struct {
	closeState CloseState
}

func NewControlFrameHandler() *ControlFrameHandler {
	return &ControlFrameHandler{}
}

func (h *ControlFrameHandler) CloseState() CloseState {
	return h.closeState
}

func (h *ControlFrameHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	frame, ok := msg.(Frame)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	switch frame.Opcode {
	case OpcodePing:
		if h.closeState != CloseStateOpen {
			releaseFrame(frame)
			return
		}
		if err := ctx.Channel().WriteAndFlush(Frame{Final: true, Opcode: OpcodePong, Payload: frame.Payload}); err != nil {
			releaseFrame(Frame{Payload: frame.Payload})
			ctx.FireExceptionCaught(err)
		}
	case OpcodeClose:
		h.readClose(ctx, frame)
	default:
		ctx.FireChannelRead(frame)
	}
}

func (h *ControlFrameHandler) Write(ctx *channel.HandlerContext, msg any) error {
	frame, ok := msg.(Frame)
	if !ok {
		return ctx.Write(msg)
	}
	if frame.Opcode != OpcodeClose {
		if h.closeState != CloseStateOpen {
			releaseFrame(frame)
			return ErrCloseHandshakeInProgress
		}
		return ctx.Write(frame)
	}
	return h.writeClose(ctx, frame, false)
}

func (h *ControlFrameHandler) Close(ctx *channel.HandlerContext) error {
	if h.closeState == CloseStateOpen {
		frame, err := NewCloseFrame(ctx, CloseStatusNormalClosure, "")
		if err != nil {
			return err
		}
		if err := h.writeClose(ctx, frame, false); err != nil {
			return err
		}
		if err := ctx.Flush(); err != nil {
			return err
		}
	}
	h.closeState = CloseStateClosed
	return ctx.Close()
}

func (h *ControlFrameHandler) readClose(ctx *channel.HandlerContext, frame Frame) {
	status, _ := ParseCloseStatus(frame)
	switch h.closeState {
	case CloseStateOpen:
		h.closeState = CloseStateCloseReceived
		ctx.FireUserEventTriggered(CloseStateEvent{State: h.closeState, Status: status, Remote: true})
		if err := h.writeClose(ctx, Frame{Final: true, Opcode: OpcodeClose, Payload: frame.Payload}, true); err != nil {
			releaseFrame(Frame{Payload: frame.Payload})
			h.closeState = CloseStateClosed
			ctx.FireExceptionCaught(err)
			_ = ctx.Close()
			return
		}
		if err := ctx.Flush(); err != nil {
			h.closeState = CloseStateClosed
			ctx.FireExceptionCaught(err)
			_ = ctx.Close()
			return
		}
		frame.Payload = nil
		_ = ctx.Close()
	case CloseStateCloseSent:
		h.closeState = CloseStateClosed
		ctx.FireUserEventTriggered(CloseStateEvent{State: h.closeState, Status: status, Remote: true})
		releaseFrame(frame)
		_ = ctx.Close()
	default:
		releaseFrame(frame)
	}
}

func (h *ControlFrameHandler) writeClose(ctx *channel.HandlerContext, frame Frame, remote bool) error {
	switch h.closeState {
	case CloseStateOpen:
		h.closeState = CloseStateCloseSent
		ctx.FireUserEventTriggered(CloseStateEvent{State: h.closeState, Remote: remote})
	case CloseStateCloseReceived:
		h.closeState = CloseStateClosed
		ctx.FireUserEventTriggered(CloseStateEvent{State: h.closeState, Remote: remote})
	case CloseStateCloseSent, CloseStateClosed:
		releaseFrame(frame)
		return ErrCloseHandshakeInProgress
	}
	return ctx.Write(frame)
}

type CloseStatus struct {
	Code   uint16
	Reason string
}

func NewCloseFrame(ctx *channel.HandlerContext, code uint16, reason string) (Frame, error) {
	if !IsValidCloseStatusCode(code) {
		return Frame{}, ErrCloseStatusInvalid
	}
	size := 2 + len(reason)
	if size > 125 {
		return Frame{}, ErrControlFrameInvalid
	}
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

type utf8State struct {
	need  int
	lower byte
	upper byte
}

func (s *utf8State) reset() {
	s.need = 0
	s.lower = 0x80
	s.upper = 0xbf
}

func (s *utf8State) accept(b byte) bool {
	if s.need > 0 {
		if b < s.lower || b > s.upper {
			return false
		}
		s.need--
		s.lower = 0x80
		s.upper = 0xbf
		return true
	}
	switch {
	case b < 0x80:
		return true
	case b >= 0xc2 && b <= 0xdf:
		s.need = 1
	case b == 0xe0:
		s.need = 2
		s.lower = 0xa0
	case b >= 0xe1 && b <= 0xec:
		s.need = 2
	case b == 0xed:
		s.need = 2
		s.upper = 0x9f
	case b >= 0xee && b <= 0xef:
		s.need = 2
	case b == 0xf0:
		s.need = 3
		s.lower = 0x90
	case b >= 0xf1 && b <= 0xf3:
		s.need = 3
	case b == 0xf4:
		s.need = 3
		s.upper = 0x8f
	default:
		return false
	}
	return true
}

// UTF8Validator 对 Text frame 执行流式 UTF-8 校验，不复制 payload。
type UTF8Validator struct {
	fragmentOpcode byte
	utf8           utf8State
}

func NewUTF8Validator() *UTF8Validator {
	h := &UTF8Validator{}
	h.utf8.reset()
	return h
}

func (h *UTF8Validator) ChannelRead(ctx *channel.HandlerContext, msg any) {
	frame, ok := msg.(Frame)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	switch frame.Opcode {
	case OpcodeText:
		if h.fragmentOpcode != 0 {
			h.fail(ctx, frame)
			return
		}
		h.utf8.reset()
		if !h.validate(frame.Payload, frame.Final) {
			h.fail(ctx, frame)
			return
		}
		if !frame.Final {
			h.fragmentOpcode = OpcodeText
		}
	case OpcodeBinary:
		if !frame.Final {
			h.fragmentOpcode = OpcodeBinary
		}
	case OpcodeContinuation:
		if h.fragmentOpcode == OpcodeText && !h.validate(frame.Payload, frame.Final) {
			h.fail(ctx, frame)
			return
		}
		if frame.Final {
			h.fragmentOpcode = 0
			h.utf8.reset()
		}
	}
	ctx.FireChannelRead(frame)
}

func (h *UTF8Validator) ChannelInactive(ctx *channel.HandlerContext) {
	h.fragmentOpcode = 0
	h.utf8.reset()
	ctx.FireChannelInactive()
}

func (h *UTF8Validator) validate(payload buffer.ByteBuf, final bool) bool {
	if payload != nil {
		var inline [8][]byte
		for _, part := range payload.ReadableSlices(inline[:0]) {
			for _, b := range part {
				if !h.utf8.accept(b) {
					return false
				}
			}
		}
	}
	return !final || h.utf8.need == 0
}

func (h *UTF8Validator) fail(ctx *channel.HandlerContext, frame Frame) {
	releaseFrame(frame)
	closeFrame, err := NewCloseFrame(ctx, CloseStatusInvalidFrameData, "invalid utf-8")
	if err == nil {
		_ = ctx.Channel().WriteAndFlush(closeFrame)
	}
	ctx.FireExceptionCaught(ErrInvalidUTF8)
	_ = ctx.Close()
	h.fragmentOpcode = 0
	h.utf8.reset()
}

// IdleHandler 将 timeout.IdleStateEvent 映射为 WebSocket ping 或 close。
type IdleHandler struct {
	pingPayload []byte
	closeCode   uint16
	closeReason string
}

func NewIdleHandler(pingPayload []byte, closeCode uint16, closeReason string) *IdleHandler {
	if closeCode == 0 {
		closeCode = CloseStatusGoingAway
	}
	return &IdleHandler{
		pingPayload: append([]byte(nil), pingPayload...),
		closeCode:   closeCode,
		closeReason: closeReason,
	}
}

func (h *IdleHandler) UserEventTriggered(ctx *channel.HandlerContext, event any) {
	idle, ok := event.(timeout.IdleStateEvent)
	if !ok {
		ctx.FireUserEventTriggered(event)
		return
	}
	switch idle.State {
	case timeout.WriterIdle, timeout.AllIdle:
		h.writePing(ctx)
	case timeout.ReaderIdle:
		h.writeClose(ctx)
	default:
		ctx.FireUserEventTriggered(event)
	}
}

func (h *IdleHandler) writePing(ctx *channel.HandlerContext) {
	payload, err := copyPayload(ctx, h.pingPayload)
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Channel().WriteAndFlush(Frame{Final: true, Opcode: OpcodePing, Payload: payload}); err != nil {
		if payload != nil {
			payload.Release()
		}
		ctx.FireExceptionCaught(err)
	}
}

func (h *IdleHandler) writeClose(ctx *channel.HandlerContext) {
	frame, err := NewCloseFrame(ctx, h.closeCode, h.closeReason)
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Channel().WriteAndFlush(frame); err != nil {
		releaseFrame(frame)
		ctx.FireExceptionCaught(err)
		return
	}
	_ = ctx.Close()
}

func copyPayload(ctx *channel.HandlerContext, payload []byte) (buffer.ByteBuf, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	out, err := ctx.Channel().Allocator().Acquire(len(payload))
	if err != nil {
		return nil, err
	}
	if _, err := out.WriteBytes(payload); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

func releaseFrame(frame Frame) {
	if frame.Payload != nil {
		frame.Payload.Release()
	}
}

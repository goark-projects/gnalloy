package quic

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/udp"
)

// RuntimeHandlerConfig 描述 QUIC runtime handler 的本地策略。
type RuntimeHandlerConfig struct {
	// Runtime 是首次创建 Connection Runtime 时使用的参数。
	Runtime RuntimeConfig
	// AllowDuplicatePackets 为 true 时不丢弃重复 ack-eliciting packet，仅用于诊断。
	AllowDuplicatePackets bool
}

// RuntimeHandler 把 frame 事件应用到连接 runtime，再继续向后传播。
type RuntimeHandler struct {
	cfg  RuntimeHandlerConfig
	last packetObservation
}

type packetObservation struct {
	conn     *Connection
	space    PacketNumberSpace
	number   uint64
	accepted bool
	valid    bool
}

// NewRuntimeHandler 创建 QUIC runtime handler。
func NewRuntimeHandler(cfg RuntimeHandlerConfig) *RuntimeHandler {
	return &RuntimeHandler{cfg: cfg}
}

// ChannelRead 应用 frame 对 ACK、loss、stream、path 和连接状态的影响。
func (h *RuntimeHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if addressed, ok := asUDPAddressed(msg); ok {
		h.handleAddressed(ctx, addressed)
		return
	}
	if event, ok := asFrameEvent(msg); ok {
		h.handleFrameEvent(ctx, event)
		return
	}
	ctx.FireChannelRead(msg)
}

func (h *RuntimeHandler) handleAddressed(ctx *channel.HandlerContext, addressed udp.Addressed) {
	event, ok := asFrameEvent(addressed.Message)
	if !ok {
		ctx.FireChannelRead(addressed)
		return
	}
	accepted, err := h.apply(event)
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if !accepted {
		return
	}
	ctx.FireChannelRead(udp.Addressed{Message: event, Addr: addressed.Addr})
}

func (h *RuntimeHandler) handleFrameEvent(ctx *channel.HandlerContext, event FrameEvent) {
	accepted, err := h.apply(event)
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if !accepted {
		return
	}
	ctx.FireChannelRead(event)
}

func (h *RuntimeHandler) apply(event FrameEvent) (bool, error) {
	if event.Conn == nil {
		return true, nil
	}
	runtime, err := event.Conn.RuntimeWithConfig(h.cfg.Runtime)
	if err != nil {
		event.Release()
		return false, err
	}
	if isAckElicitingFrame(event.Frame) && !h.cfg.AllowDuplicatePackets {
		if !h.acceptPacket(runtime, event) {
			event.Release()
			return false, nil
		}
	}
	if event.Conn.State == ConnectionStateNew {
		event.Conn.State = ConnectionStateActive
	}
	if err := runtime.ApplyFrameFrom(event.Remote, event.Packet.Space, event.Frame); err != nil {
		event.Release()
		return false, err
	}
	return true, nil
}

func (h *RuntimeHandler) acceptPacket(runtime *Runtime, event FrameEvent) bool {
	if event.Packet.FrameIndex > 0 && h.last.matches(event.Conn, event.Packet.Space, event.Packet.PacketNumber) {
		return h.last.accepted
	}
	accepted := runtime.ObservePacket(event.Packet.Space, event.Packet.PacketNumber)
	h.last = packetObservation{
		conn:     event.Conn,
		space:    event.Packet.Space,
		number:   event.Packet.PacketNumber,
		accepted: accepted,
		valid:    true,
	}
	return accepted
}

func (o packetObservation) matches(conn *Connection, space PacketNumberSpace, number uint64) bool {
	return o.valid && o.conn == conn && o.space == space && o.number == number
}

func isAckElicitingFrame(frame any) bool {
	switch frame.(type) {
	case PaddingFrame, ACKFrame, ConnectionCloseFrame:
		return false
	default:
		return true
	}
}

func asFrameEvent(msg any) (FrameEvent, bool) {
	switch v := msg.(type) {
	case FrameEvent:
		return v, true
	case *FrameEvent:
		if v == nil {
			return FrameEvent{}, false
		}
		return *v, true
	default:
		return FrameEvent{}, false
	}
}

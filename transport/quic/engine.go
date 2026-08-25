package quic

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/udp"
)

// PacketEvent 是 QUIC packet engine 路由后的入站事件。
type PacketEvent struct {
	Packet        Packet
	Conn          *Connection
	Remote        udp.Address
	NewConnection bool
}

func (e PacketEvent) Release() {
	e.Packet.Release()
}

// PacketHandlerConfig 配置 UDP 数据报与 QUIC 包之间的最小引擎。
type PacketHandlerConfig struct {
	HeaderParseOptions HeaderParseOptions
	Router             *ConnectionIDRouter
}

// PacketHandler 把 UDP 数据报提升为带连接上下文的 QUIC 包事件。
type PacketHandler struct {
	opts   HeaderParseOptions
	router *ConnectionIDRouter
}

func NewPacketHandler(cfg PacketHandlerConfig) *PacketHandler {
	router := cfg.Router
	if router == nil {
		router = NewConnectionIDRouter(0)
	}
	return &PacketHandler{opts: cfg.HeaderParseOptions, router: router}
}

func (h *PacketHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch v := msg.(type) {
	case udp.Datagram:
		h.decodeDatagram(ctx, v)
	case *udp.Datagram:
		if v == nil {
			ctx.FireChannelRead(msg)
			return
		}
		h.decodeDatagram(ctx, *v)
	case udp.Addressed:
		h.decodeAddressed(ctx, v)
	case *udp.Addressed:
		if v == nil {
			ctx.FireChannelRead(msg)
			return
		}
		h.decodeAddressed(ctx, *v)
	default:
		ctx.FireChannelRead(msg)
	}
}

func (h *PacketHandler) Write(ctx *channel.HandlerContext, msg any) error {
	addressed, ok := asUDPAddressed(msg)
	if !ok {
		return ctx.Write(msg)
	}
	packet, ok := asPacket(addressed.Message)
	if !ok {
		return ctx.Write(msg)
	}
	out, err := EncodePacket(ctx.Channel().Allocator(), packet)
	packet.Release()
	if err != nil {
		return err
	}
	if err := ctx.Write(udp.Datagram{Payload: out, Addr: addressed.Addr}); err != nil {
		out.Release()
		return err
	}
	return nil
}

func (h *PacketHandler) decodeDatagram(ctx *channel.HandlerContext, datagram udp.Datagram) {
	packet, err := DecodePacket(datagram.Payload, h.opts)
	datagram.Release()
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	h.routePacket(ctx, packet, datagram.Addr)
}

func (h *PacketHandler) decodeAddressed(ctx *channel.HandlerContext, addressed udp.Addressed) {
	switch msg := addressed.Message.(type) {
	case buffer.ByteBuf:
		packet, err := DecodePacket(msg, h.opts)
		addressed.Release()
		if err != nil {
			ctx.FireExceptionCaught(err)
			return
		}
		h.routePacket(ctx, packet, addressed.Addr)
	case Packet:
		h.routePacket(ctx, msg, addressed.Addr)
	case *Packet:
		if msg == nil {
			ctx.FireChannelRead(addressed)
			return
		}
		h.routePacket(ctx, *msg, addressed.Addr)
	default:
		ctx.FireChannelRead(addressed)
	}
}

func (h *PacketHandler) routePacket(ctx *channel.HandlerContext, packet Packet, remote udp.Address) {
	conn, created, err := h.router.Route(packet, remote)
	if err != nil {
		packet.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if !created && conn.State == ConnectionStateNew {
		conn.State = ConnectionStateActive
	}
	ctx.FireChannelRead(PacketEvent{
		Packet:        packet,
		Conn:          conn,
		Remote:        remote,
		NewConnection: created,
	})
}

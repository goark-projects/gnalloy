package ip

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/raw"
)

type Decoder struct{}

func NewDecoder() *Decoder {
	return &Decoder{}
}

func (d *Decoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch v := msg.(type) {
	case raw.Packet:
		d.decodeRawPacket(ctx, v)
	case *raw.Packet:
		if v == nil {
			ctx.FireChannelRead(msg)
			return
		}
		d.decodeRawPacket(ctx, *v)
	case raw.Addressed:
		d.decodeAddressed(ctx, v)
	case *raw.Addressed:
		if v == nil {
			ctx.FireChannelRead(msg)
			return
		}
		d.decodeAddressed(ctx, *v)
	default:
		ctx.FireChannelRead(msg)
	}
}

func (d *Decoder) decodeRawPacket(ctx *channel.HandlerContext, packet raw.Packet) {
	decoded, err := DecodePacket(packet.Payload)
	packet.Release()
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(raw.Addressed{Message: decoded, Addr: packet.Addr, Protocol: decoded.Header.PayloadProtocol()})
}

func (d *Decoder) decodeAddressed(ctx *channel.HandlerContext, addressed raw.Addressed) {
	payload, ok := addressed.Message.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(addressed)
		return
	}
	decoded, err := DecodePacket(payload)
	addressed.Release()
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(raw.Addressed{Message: decoded, Addr: addressed.Addr, Protocol: decoded.Header.PayloadProtocol()})
}

type Encoder struct{}

func NewEncoder() *Encoder {
	return &Encoder{}
}

func (e *Encoder) Write(ctx *channel.HandlerContext, msg any) error {
	addressed, ok := asRawAddressed(msg)
	if !ok {
		return ctx.Write(msg)
	}
	packet, ok := addressed.Message.(Packet)
	if !ok {
		if ptr, ptrOK := addressed.Message.(*Packet); ptrOK && ptr != nil {
			packet = *ptr
			ok = true
		}
	}
	if !ok {
		return ctx.Write(msg)
	}
	out, err := EncodePacket(ctx.Channel().Allocator(), packet)
	packet.Release()
	if err != nil {
		return err
	}
	protocol := addressed.Protocol
	if protocol == 0 {
		protocol = packet.Header.PayloadProtocol()
	}
	if err := ctx.Write(raw.Addressed{Message: out, Addr: addressed.Addr, Protocol: protocol}); err != nil {
		out.Release()
		return err
	}
	return nil
}

func asRawAddressed(msg any) (raw.Addressed, bool) {
	switch v := msg.(type) {
	case raw.Addressed:
		return v, true
	case *raw.Addressed:
		if v == nil {
			return raw.Addressed{}, false
		}
		return *v, true
	default:
		return raw.Addressed{}, false
	}
}

package raw

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

// Addressed 是 raw 责任链中保留远端地址和协议号的 typed message 包装。
type Addressed struct {
	Message  any
	Addr     Address
	Protocol int
}

func (m Addressed) Release() {
	releaseMessage(m.Message)
}

type PacketPayloadDecoder interface {
	AcceptPacket(packet Packet) bool
	DecodePacket(ctx *channel.HandlerContext, payload buffer.ByteBuf, out *codec.MessageList) error
}

// PacketToMessageDecoder 对齐 Netty 的 MessageToMessageDecoder，但输入是单个 raw packet。
type PacketToMessageDecoder struct {
	decoder PacketPayloadDecoder
	out     codec.MessageList
}

func NewPacketToMessageDecoder(decoder PacketPayloadDecoder) *PacketToMessageDecoder {
	return &PacketToMessageDecoder{decoder: decoder}
}

func (d *PacketToMessageDecoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	packet, ok := asPacket(msg)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if d.decoder == nil {
		packet.Release()
		ctx.FireExceptionCaught(codec.ErrInvalidDecoder)
		return
	}
	if !d.decoder.AcceptPacket(packet) {
		ctx.FireChannelRead(msg)
		return
	}
	d.out.Reset()
	err := d.decoder.DecodePacket(ctx, packet.Payload, &d.out)
	packet.Release()
	if err != nil {
		d.out.ReleaseAll()
		ctx.FireExceptionCaught(err)
		return
	}
	for i := 0; i < d.out.Len(); i++ {
		ctx.FireChannelRead(Addressed{Message: d.out.At(i), Addr: packet.Addr, Protocol: packet.Protocol})
	}
	d.out.Reset()
}

type packetPayloadDecoderFunc struct {
	accept func(Packet) bool
	decode func(*channel.HandlerContext, buffer.ByteBuf, *codec.MessageList) error
}

func NewPacketToMessageDecoderFunc(accept func(Packet) bool, decode func(*channel.HandlerContext, buffer.ByteBuf, *codec.MessageList) error) *PacketToMessageDecoder {
	return NewPacketToMessageDecoder(packetPayloadDecoderFunc{accept: accept, decode: decode})
}

func (f packetPayloadDecoderFunc) AcceptPacket(packet Packet) bool {
	return f.accept == nil || f.accept(packet)
}

func (f packetPayloadDecoderFunc) DecodePacket(ctx *channel.HandlerContext, payload buffer.ByteBuf, out *codec.MessageList) error {
	if f.decode == nil {
		return codec.ErrInvalidDecoder
	}
	return f.decode(ctx, payload, out)
}

type AddressedMessageEncoder interface {
	AcceptAddressedMessage(msg any) bool
	EncodeAddressed(ctx *channel.HandlerContext, msg any, out *codec.MessageList) error
}

// MessageToPacketEncoder 把 Addressed typed message 编码成一个或多个 raw packet。
type MessageToPacketEncoder struct {
	encoder AddressedMessageEncoder
	out     codec.MessageList
}

func NewMessageToPacketEncoder(encoder AddressedMessageEncoder) *MessageToPacketEncoder {
	return &MessageToPacketEncoder{encoder: encoder}
}

func (e *MessageToPacketEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	addressed, ok := asAddressed(msg)
	if !ok {
		return ctx.Write(msg)
	}
	if e.encoder == nil {
		addressed.Release()
		return codec.ErrInvalidEncoder
	}
	if !e.encoder.AcceptAddressedMessage(addressed.Message) {
		payload, ok := addressed.Message.(buffer.ByteBuf)
		if !ok {
			return ctx.Write(msg)
		}
		if err := ctx.Write(Packet{Payload: payload, Addr: addressed.Addr, Protocol: addressed.Protocol}); err != nil {
			payload.Release()
			return err
		}
		return nil
	}
	e.out.Reset()
	err := e.encoder.EncodeAddressed(ctx, addressed.Message, &e.out)
	releaseMessage(addressed.Message)
	if err != nil {
		e.out.ReleaseAll()
		return err
	}
	for i := 0; i < e.out.Len(); i++ {
		payload, ok := e.out.At(i).(buffer.ByteBuf)
		if !ok {
			for j := i; j < e.out.Len(); j++ {
				releaseMessage(e.out.At(j))
			}
			e.out.Reset()
			return ErrInvalidPacket
		}
		if err := ctx.Write(Packet{Payload: payload, Addr: addressed.Addr, Protocol: addressed.Protocol}); err != nil {
			for j := i; j < e.out.Len(); j++ {
				releaseMessage(e.out.At(j))
			}
			e.out.Reset()
			return err
		}
	}
	e.out.Reset()
	return nil
}

type addressedMessageEncoderFunc struct {
	accept func(any) bool
	encode func(*channel.HandlerContext, any, *codec.MessageList) error
}

func NewMessageToPacketEncoderFunc(accept func(any) bool, encode func(*channel.HandlerContext, any, *codec.MessageList) error) *MessageToPacketEncoder {
	return NewMessageToPacketEncoder(addressedMessageEncoderFunc{accept: accept, encode: encode})
}

func (f addressedMessageEncoderFunc) AcceptAddressedMessage(msg any) bool {
	return f.accept == nil || f.accept(msg)
}

func (f addressedMessageEncoderFunc) EncodeAddressed(ctx *channel.HandlerContext, msg any, out *codec.MessageList) error {
	if f.encode == nil {
		return codec.ErrInvalidEncoder
	}
	return f.encode(ctx, msg, out)
}

func asAddressed(msg any) (Addressed, bool) {
	switch v := msg.(type) {
	case Addressed:
		return v, true
	case *Addressed:
		if v == nil {
			return Addressed{}, false
		}
		return *v, true
	default:
		return Addressed{}, false
	}
}

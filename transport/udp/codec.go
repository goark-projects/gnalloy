package udp

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

// Addressed 是 UDP 责任链中保留远端地址的 typed message 包装。
type Addressed struct {
	Message any
	Addr    Address
}

func (m Addressed) Release() {
	releaseMessage(m.Message)
}

type DatagramPayloadDecoder interface {
	AcceptDatagram(datagram Datagram) bool
	DecodeDatagram(ctx *channel.HandlerContext, payload buffer.ByteBuf, out *codec.MessageList) error
}

// DatagramToMessageDecoder 对齐 Netty 的 MessageToMessageDecoder，但输入是单个 UDP datagram。
type DatagramToMessageDecoder struct {
	decoder DatagramPayloadDecoder
	out     codec.MessageList
}

func NewDatagramToMessageDecoder(decoder DatagramPayloadDecoder) *DatagramToMessageDecoder {
	return &DatagramToMessageDecoder{decoder: decoder}
}

func (d *DatagramToMessageDecoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	datagram, ok := asDatagram(msg)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if d.decoder == nil {
		datagram.Release()
		ctx.FireExceptionCaught(codec.ErrInvalidDecoder)
		return
	}
	if !d.decoder.AcceptDatagram(datagram) {
		ctx.FireChannelRead(msg)
		return
	}
	d.out.Reset()
	err := d.decoder.DecodeDatagram(ctx, datagram.Payload, &d.out)
	datagram.Release()
	if err != nil {
		d.out.ReleaseAll()
		ctx.FireExceptionCaught(err)
		return
	}
	for i := 0; i < d.out.Len(); i++ {
		ctx.FireChannelRead(Addressed{Message: d.out.At(i), Addr: datagram.Addr})
	}
	d.out.Reset()
}

type datagramPayloadDecoderFunc struct {
	accept func(Datagram) bool
	decode func(*channel.HandlerContext, buffer.ByteBuf, *codec.MessageList) error
}

func NewDatagramToMessageDecoderFunc(accept func(Datagram) bool, decode func(*channel.HandlerContext, buffer.ByteBuf, *codec.MessageList) error) *DatagramToMessageDecoder {
	return NewDatagramToMessageDecoder(datagramPayloadDecoderFunc{accept: accept, decode: decode})
}

func (f datagramPayloadDecoderFunc) AcceptDatagram(datagram Datagram) bool {
	return f.accept == nil || f.accept(datagram)
}

func (f datagramPayloadDecoderFunc) DecodeDatagram(ctx *channel.HandlerContext, payload buffer.ByteBuf, out *codec.MessageList) error {
	if f.decode == nil {
		return codec.ErrInvalidDecoder
	}
	return f.decode(ctx, payload, out)
}

type AddressedMessageEncoder interface {
	AcceptAddressedMessage(msg any) bool
	EncodeAddressed(ctx *channel.HandlerContext, msg any, out *codec.MessageList) error
}

// MessageToDatagramEncoder 把 Addressed typed message 编码成一个或多个 UDP datagram。
type MessageToDatagramEncoder struct {
	encoder AddressedMessageEncoder
	out     codec.MessageList
}

func NewMessageToDatagramEncoder(encoder AddressedMessageEncoder) *MessageToDatagramEncoder {
	return &MessageToDatagramEncoder{encoder: encoder}
}

func (e *MessageToDatagramEncoder) Write(ctx *channel.HandlerContext, msg any) error {
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
		if err := ctx.Write(Datagram{Payload: payload, Addr: addressed.Addr}); err != nil {
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
			return ErrInvalidDatagram
		}
		if err := ctx.Write(Datagram{Payload: payload, Addr: addressed.Addr}); err != nil {
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

func NewMessageToDatagramEncoderFunc(accept func(any) bool, encode func(*channel.HandlerContext, any, *codec.MessageList) error) *MessageToDatagramEncoder {
	return NewMessageToDatagramEncoder(addressedMessageEncoderFunc{accept: accept, encode: encode})
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

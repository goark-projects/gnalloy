package ip

import (
	"net"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/transport/raw"
)

type ProtocolPayloadDecoder interface {
	AcceptProtocol(protocol int) bool
	DecodePayload(ctx *channel.HandlerContext, header Header, payload buffer.ByteBuf, out *codec.MessageList) error
}

type ProtocolPayloadDecoderHandler struct {
	decoder ProtocolPayloadDecoder
	out     codec.MessageList
}

func NewProtocolPayloadDecoder(decoder ProtocolPayloadDecoder) *ProtocolPayloadDecoderHandler {
	return &ProtocolPayloadDecoderHandler{decoder: decoder}
}

func (d *ProtocolPayloadDecoderHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	addressed, ok := asRawAddressed(msg)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	packet, ok := asIPPacket(addressed.Message)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	protocol := packet.Header.PayloadProtocol()
	if d.decoder == nil || !d.decoder.AcceptProtocol(protocol) {
		ctx.FireChannelRead(msg)
		return
	}
	d.out.Reset()
	err := d.decoder.DecodePayload(ctx, packet.Header, packet.Payload, &d.out)
	addressed.Release()
	if err != nil {
		d.out.ReleaseAll()
		ctx.FireExceptionCaught(err)
		return
	}
	for i := 0; i < d.out.Len(); i++ {
		ctx.FireChannelRead(raw.Addressed{Message: d.out.At(i), Addr: addressed.Addr, Protocol: protocol})
	}
	d.out.Reset()
}

type protocolPayloadDecoderFunc struct {
	accept func(int) bool
	decode func(*channel.HandlerContext, Header, buffer.ByteBuf, *codec.MessageList) error
}

func NewProtocolPayloadDecoderFunc(accept func(int) bool, decode func(*channel.HandlerContext, Header, buffer.ByteBuf, *codec.MessageList) error) *ProtocolPayloadDecoderHandler {
	return NewProtocolPayloadDecoder(protocolPayloadDecoderFunc{accept: accept, decode: decode})
}

func (f protocolPayloadDecoderFunc) AcceptProtocol(protocol int) bool {
	return f.accept == nil || f.accept(protocol)
}

func (f protocolPayloadDecoderFunc) DecodePayload(ctx *channel.HandlerContext, header Header, payload buffer.ByteBuf, out *codec.MessageList) error {
	if f.decode == nil {
		return codec.ErrInvalidDecoder
	}
	return f.decode(ctx, header, payload, out)
}

type ProtocolPayloadEncoderConfig struct {
	Version  int
	Source   net.IP
	TTL      uint8
	HopLimit uint8
}

type ProtocolPayloadEncoder interface {
	AcceptOutboundMessage(msg any) bool
	EncodePayload(ctx *channel.HandlerContext, msg any, out *codec.MessageList) error
}

type ProtocolPayloadEncoderHandler struct {
	cfg     ProtocolPayloadEncoderConfig
	encoder ProtocolPayloadEncoder
	out     codec.MessageList
}

func NewProtocolPayloadEncoder(cfg ProtocolPayloadEncoderConfig, encoder ProtocolPayloadEncoder) *ProtocolPayloadEncoderHandler {
	return &ProtocolPayloadEncoderHandler{cfg: normalizeProtocolPayloadEncoderConfig(cfg), encoder: encoder}
}

func (e *ProtocolPayloadEncoderHandler) Write(ctx *channel.HandlerContext, msg any) error {
	addressed, ok := asRawAddressed(msg)
	if !ok {
		return ctx.Write(msg)
	}
	if e.encoder == nil || !e.encoder.AcceptOutboundMessage(addressed.Message) {
		return ctx.Write(msg)
	}
	e.out.Reset()
	err := e.encoder.EncodePayload(ctx, addressed.Message, &e.out)
	releaseProtocolMessage(addressed.Message)
	if err != nil {
		e.out.ReleaseAll()
		return err
	}
	for i := 0; i < e.out.Len(); i++ {
		payload, ok := e.out.At(i).(buffer.ByteBuf)
		if !ok {
			for j := i; j < e.out.Len(); j++ {
				releaseProtocolMessage(e.out.At(j))
			}
			e.out.Reset()
			return ErrInvalidPacket
		}
		packet := Packet{Header: e.headerFor(addressed, payload), Payload: payload}
		if err := ctx.Write(raw.Addressed{Message: packet, Addr: addressed.Addr, Protocol: raw.ProtocolRaw}); err != nil {
			packet.Release()
			for j := i; j < e.out.Len(); j++ {
				releaseProtocolMessage(e.out.At(j))
			}
			e.out.Reset()
			return err
		}
	}
	e.out.Reset()
	return nil
}

type protocolPayloadEncoderFunc struct {
	accept func(any) bool
	encode func(*channel.HandlerContext, any, *codec.MessageList) error
}

func NewProtocolPayloadEncoderFunc(cfg ProtocolPayloadEncoderConfig, accept func(any) bool, encode func(*channel.HandlerContext, any, *codec.MessageList) error) *ProtocolPayloadEncoderHandler {
	return NewProtocolPayloadEncoder(cfg, protocolPayloadEncoderFunc{accept: accept, encode: encode})
}

func (f protocolPayloadEncoderFunc) AcceptOutboundMessage(msg any) bool {
	return f.accept == nil || f.accept(msg)
}

func (f protocolPayloadEncoderFunc) EncodePayload(ctx *channel.HandlerContext, msg any, out *codec.MessageList) error {
	if f.encode == nil {
		return codec.ErrInvalidEncoder
	}
	return f.encode(ctx, msg, out)
}

func normalizeProtocolPayloadEncoderConfig(cfg ProtocolPayloadEncoderConfig) ProtocolPayloadEncoderConfig {
	if cfg.Version == 0 {
		cfg.Version = Version4
	}
	if cfg.TTL == 0 {
		cfg.TTL = DefaultTTL
	}
	if cfg.HopLimit == 0 {
		cfg.HopLimit = DefaultHopLimit
	}
	return cfg
}

func (e *ProtocolPayloadEncoderHandler) headerFor(addressed raw.Addressed, payload buffer.ByteBuf) Header {
	version := e.cfg.Version
	if addressed.Addr.IP != nil && addressed.Addr.IP.To4() == nil {
		version = Version6
	}
	header := Header{
		Version:       version,
		TTL:           e.cfg.TTL,
		HopLimit:      e.cfg.HopLimit,
		Source:        e.cfg.Source,
		Destination:   addressed.Addr.IP,
		Protocol:      addressed.Protocol,
		NextHeader:    addressed.Protocol,
		PayloadLength: payload.ReadableBytes(),
	}
	return header
}

func asIPPacket(msg any) (Packet, bool) {
	switch v := msg.(type) {
	case Packet:
		return v, true
	case *Packet:
		if v == nil {
			return Packet{}, false
		}
		return *v, true
	default:
		return Packet{}, false
	}
}

func releaseProtocolMessage(msg any) {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		buf.Release()
		return
	}
	if releasable, ok := msg.(interface{ Release() }); ok {
		releasable.Release()
	}
}

package ip

import (
	"net"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/internal/message"
	"goark.dev/gnalloy/transport/raw"
)

type ProtocolPayloadDecoder interface {
	AcceptProtocol(protocol int) bool
	DecodePayload(ctx *channel.HandlerContext, header Header, payload buffer.ByteBuf, out *codec.MessageList) error
}

// PacketFormat 描述 raw.Packet.Payload 的线缆格式。
type PacketFormat uint8

const (
	// PacketFormatPayload 表示 raw payload 已经是指定 IP 协议的负载。
	PacketFormatPayload PacketFormat = iota
	// PacketFormatIP 表示 raw payload 是完整 IPv4/IPv6 packet。
	PacketFormatIP
)

// ProtocolCodecConfig 描述自定义 IP 协议 codec 的入出站格式。
type ProtocolCodecConfig struct {
	Protocol     int
	PacketFormat PacketFormat
	Version      int
	Source       net.IP
	TTL          uint8
	HopLimit     uint8
}

// ProtocolCodec 将 raw.Packet 与自定义 IP 协议消息做双工转换。
type ProtocolCodec struct {
	cfg     ProtocolCodecConfig
	decoder ProtocolPayloadDecoder
	encoder ProtocolPayloadEncoder
	out     codec.MessageList
}

func NewProtocolCodec(cfg ProtocolCodecConfig, decoder ProtocolPayloadDecoder, encoder ProtocolPayloadEncoder) *ProtocolCodec {
	return &ProtocolCodec{cfg: normalizeProtocolCodecConfig(cfg), decoder: decoder, encoder: encoder}
}

func NewProtocolCodecFunc(
	cfg ProtocolCodecConfig,
	acceptProtocol func(int) bool,
	decode func(*channel.HandlerContext, Header, buffer.ByteBuf, *codec.MessageList) error,
	acceptOutbound func(any) bool,
	encode func(*channel.HandlerContext, any, *codec.MessageList) error,
) *ProtocolCodec {
	var decoder ProtocolPayloadDecoder
	if acceptProtocol != nil || decode != nil {
		decoder = protocolPayloadDecoderFunc{accept: acceptProtocol, decode: decode}
	}
	var encoder ProtocolPayloadEncoder
	if acceptOutbound != nil || encode != nil {
		encoder = protocolPayloadEncoderFunc{accept: acceptOutbound, encode: encode}
	}
	return NewProtocolCodec(cfg, decoder, encoder)
}

func (c *ProtocolCodec) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch v := msg.(type) {
	case raw.Packet:
		c.decodeRawPacket(ctx, msg, v)
	case *raw.Packet:
		if v == nil {
			ctx.FireChannelRead(msg)
			return
		}
		c.decodeRawPacket(ctx, msg, *v)
	case raw.Addressed:
		c.decodeAddressed(ctx, msg, v)
	case *raw.Addressed:
		if v == nil {
			ctx.FireChannelRead(msg)
			return
		}
		c.decodeAddressed(ctx, msg, *v)
	default:
		ctx.FireChannelRead(msg)
	}
}

func (c *ProtocolCodec) Write(ctx *channel.HandlerContext, msg any) error {
	addressed, ok := asRawAddressed(msg)
	if !ok {
		return ctx.Write(msg)
	}
	if c.encoder == nil || !c.encoder.AcceptOutboundMessage(addressed.Message) {
		return ctx.Write(msg)
	}
	protocol := c.outboundProtocol(addressed)
	if !validProtocol(protocol) {
		addressed.Release()
		return ErrInvalidProtocol
	}
	c.out.Reset()
	err := c.encoder.EncodePayload(ctx, addressed.Message, &c.out)
	releaseProtocolMessage(addressed.Message)
	if err != nil {
		c.out.ReleaseAll()
		return err
	}
	for i := 0; i < c.out.Len(); i++ {
		payload, ok := c.out.At(i).(buffer.ByteBuf)
		if !ok || payload == nil {
			for j := i; j < c.out.Len(); j++ {
				releaseProtocolMessage(c.out.At(j))
			}
			c.out.Reset()
			return ErrInvalidPacket
		}
		packet, err := c.rawPacket(ctx, addressed.Addr, protocol, payload)
		if err != nil {
			for j := i + 1; j < c.out.Len(); j++ {
				releaseProtocolMessage(c.out.At(j))
			}
			c.out.Reset()
			return err
		}
		if err := ctx.Write(packet); err != nil {
			packet.Release()
			for j := i + 1; j < c.out.Len(); j++ {
				releaseProtocolMessage(c.out.At(j))
			}
			c.out.Reset()
			return err
		}
	}
	c.out.Reset()
	return nil
}

func (c *ProtocolCodec) decodeRawPacket(ctx *channel.HandlerContext, original any, packet raw.Packet) {
	if c.decoder == nil {
		ctx.FireChannelRead(original)
		return
	}
	switch c.cfg.PacketFormat {
	case PacketFormatPayload:
		protocol := c.inboundProtocol(packet.Protocol)
		if !c.acceptProtocol(protocol) {
			ctx.FireChannelRead(original)
			return
		}
		header := c.payloadHeader(packet.Addr, protocol)
		c.decodePayload(ctx, header, packet.Payload, packet.Addr, protocol, packet.Release)
	case PacketFormatIP:
		decoded, err := DecodePacket(packet.Payload)
		if err != nil {
			packet.Release()
			ctx.FireExceptionCaught(err)
			return
		}
		protocol := decoded.Header.PayloadProtocol()
		if !c.acceptProtocol(protocol) {
			decoded.Release()
			ctx.FireChannelRead(original)
			return
		}
		c.decodePayload(ctx, decoded.Header, decoded.Payload, packet.Addr, protocol, func() {
			decoded.Release()
			packet.Release()
		})
	default:
		packet.Release()
		ctx.FireExceptionCaught(ErrInvalidPacket)
	}
}

func (c *ProtocolCodec) decodeAddressed(ctx *channel.HandlerContext, original any, addressed raw.Addressed) {
	if c.decoder == nil {
		ctx.FireChannelRead(original)
		return
	}
	if packet, ok := asIPPacket(addressed.Message); ok {
		protocol := packet.Header.PayloadProtocol()
		if !c.acceptProtocol(protocol) {
			ctx.FireChannelRead(original)
			return
		}
		c.decodePayload(ctx, packet.Header, packet.Payload, addressed.Addr, protocol, addressed.Release)
		return
	}
	payload, ok := addressed.Message.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(original)
		return
	}
	switch c.cfg.PacketFormat {
	case PacketFormatPayload:
		protocol := c.inboundProtocol(addressed.Protocol)
		if !c.acceptProtocol(protocol) {
			ctx.FireChannelRead(original)
			return
		}
		header := c.payloadHeader(addressed.Addr, protocol)
		c.decodePayload(ctx, header, payload, addressed.Addr, protocol, addressed.Release)
	case PacketFormatIP:
		decoded, err := DecodePacket(payload)
		if err != nil {
			addressed.Release()
			ctx.FireExceptionCaught(err)
			return
		}
		protocol := decoded.Header.PayloadProtocol()
		if !c.acceptProtocol(protocol) {
			decoded.Release()
			ctx.FireChannelRead(original)
			return
		}
		c.decodePayload(ctx, decoded.Header, decoded.Payload, addressed.Addr, protocol, func() {
			decoded.Release()
			addressed.Release()
		})
	default:
		addressed.Release()
		ctx.FireExceptionCaught(ErrInvalidPacket)
	}
}

func (c *ProtocolCodec) decodePayload(ctx *channel.HandlerContext, header Header, payload buffer.ByteBuf, addr raw.Address, protocol int, release func()) {
	c.out.Reset()
	err := c.decoder.DecodePayload(ctx, header, payload, &c.out)
	release()
	if err != nil {
		c.out.ReleaseAll()
		ctx.FireExceptionCaught(err)
		return
	}
	for i := 0; i < c.out.Len(); i++ {
		ctx.FireChannelRead(raw.Addressed{Message: c.out.At(i), Addr: addr, Protocol: protocol})
	}
	c.out.Reset()
}

func (c *ProtocolCodec) acceptProtocol(protocol int) bool {
	return validProtocol(protocol) && c.decoder != nil && c.decoder.AcceptProtocol(protocol)
}

func (c *ProtocolCodec) inboundProtocol(protocol int) int {
	if protocol == 0 {
		return c.cfg.Protocol
	}
	return protocol
}

func (c *ProtocolCodec) outboundProtocol(addressed raw.Addressed) int {
	if addressed.Protocol != 0 {
		return addressed.Protocol
	}
	return c.cfg.Protocol
}

func (c *ProtocolCodec) rawPacket(ctx *channel.HandlerContext, addr raw.Address, protocol int, payload buffer.ByteBuf) (raw.Packet, error) {
	switch c.cfg.PacketFormat {
	case PacketFormatPayload:
		return raw.Packet{Payload: payload, Addr: addr, Protocol: protocol}, nil
	case PacketFormatIP:
		packet := Packet{Header: c.headerFor(addr, protocol, payload), Payload: payload}
		encoded, err := EncodePacket(ctx.Channel().Allocator(), packet)
		payload.Release()
		if err != nil {
			return raw.Packet{}, err
		}
		return raw.Packet{Payload: encoded, Addr: addr, Protocol: protocol}, nil
	default:
		payload.Release()
		return raw.Packet{}, ErrInvalidPacket
	}
}

func (c *ProtocolCodec) payloadHeader(addr raw.Address, protocol int) Header {
	version := c.versionFor(addr)
	return Header{
		Version:    version,
		Protocol:   protocol,
		NextHeader: protocol,
		Source:     addr.IP,
		TTL:        c.cfg.TTL,
		HopLimit:   c.cfg.HopLimit,
	}
}

func (c *ProtocolCodec) headerFor(addr raw.Address, protocol int, payload buffer.ByteBuf) Header {
	version := c.versionFor(addr)
	return Header{
		Version:       version,
		Protocol:      protocol,
		NextHeader:    protocol,
		Source:        c.cfg.Source,
		Destination:   addr.IP,
		TTL:           c.cfg.TTL,
		HopLimit:      c.cfg.HopLimit,
		PayloadLength: payload.ReadableBytes(),
	}
}

func (c *ProtocolCodec) versionFor(addr raw.Address) int {
	if c.cfg.Version != 0 {
		return c.cfg.Version
	}
	if addr.IP != nil && addr.IP.To4() == nil {
		return Version6
	}
	return Version4
}

func normalizeProtocolCodecConfig(cfg ProtocolCodecConfig) ProtocolCodecConfig {
	if cfg.TTL == 0 {
		cfg.TTL = DefaultTTL
	}
	if cfg.HopLimit == 0 {
		cfg.HopLimit = DefaultHopLimit
	}
	return cfg
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
	message.Release(msg)
}

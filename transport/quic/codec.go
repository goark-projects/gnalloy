package quic

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/udp"
)

// DatagramToPacketDecoderConfig 配置 UDP payload 到 QUIC Packet 的解析行为。
type DatagramToPacketDecoderConfig struct {
	HeaderParseOptions HeaderParseOptions
}

// DatagramToPacketDecoder 把 UDP datagram 解析成 QUIC packet，保留 UDP 远端地址。
type DatagramToPacketDecoder struct {
	cfg DatagramToPacketDecoderConfig
}

func NewDatagramToPacketDecoder(cfg DatagramToPacketDecoderConfig) *DatagramToPacketDecoder {
	return &DatagramToPacketDecoder{cfg: cfg}
}

func (d *DatagramToPacketDecoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch v := msg.(type) {
	case udp.Datagram:
		d.decodeDatagram(ctx, v)
	case *udp.Datagram:
		if v == nil {
			ctx.FireChannelRead(msg)
			return
		}
		d.decodeDatagram(ctx, *v)
	case udp.Addressed:
		d.decodeAddressed(ctx, v)
	case *udp.Addressed:
		if v == nil {
			ctx.FireChannelRead(msg)
			return
		}
		d.decodeAddressed(ctx, *v)
	default:
		ctx.FireChannelRead(msg)
	}
}

func (d *DatagramToPacketDecoder) decodeDatagram(ctx *channel.HandlerContext, datagram udp.Datagram) {
	packet, err := DecodePacket(datagram.Payload, d.cfg.HeaderParseOptions)
	datagram.Release()
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(udp.Addressed{Message: packet, Addr: datagram.Addr})
}

func (d *DatagramToPacketDecoder) decodeAddressed(ctx *channel.HandlerContext, addressed udp.Addressed) {
	payload, ok := addressed.Message.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(addressed)
		return
	}
	packet, err := DecodePacket(payload, d.cfg.HeaderParseOptions)
	addressed.Release()
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(udp.Addressed{Message: packet, Addr: addressed.Addr})
}

// PacketToDatagramEncoder 把 QUIC packet 编码回 UDP datagram。
type PacketToDatagramEncoder struct{}

func NewPacketToDatagramEncoder() *PacketToDatagramEncoder {
	return &PacketToDatagramEncoder{}
}

func (e *PacketToDatagramEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	addressed, ok := asUDPAddressed(msg)
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
	if err := ctx.Write(udp.Datagram{Payload: out, Addr: addressed.Addr}); err != nil {
		out.Release()
		return err
	}
	return nil
}

// DecodePacket 解析 QUIC header，并把 payload 作为零拷贝 ByteBuf 切片挂到 Packet。
func DecodePacket(payload buffer.ByteBuf, opts HeaderParseOptions) (Packet, error) {
	header, n, err := ParseHeader(payload, opts)
	if err != nil {
		return Packet{}, err
	}
	packet := Packet{
		Header:             header,
		Type:               header.Type,
		Version:            header.Version,
		DestinationID:      header.DestinationID,
		SourceID:           header.SourceID,
		PacketNumberLength: header.PacketNumberLength,
		PacketNumber:       header.PacketNumber,
	}
	if readable := payload.WriterIndex() - n; readable > 0 {
		frame, err := payload.Slice(n, readable)
		if err != nil {
			return Packet{}, err
		}
		packet.Payload = frame
	}
	return packet, nil
}

// EncodePacket 构建连续 UDP payload。头部需要新内存，业务 payload 只做一次顺序拷贝。
func EncodePacket(alloc buffer.Allocator, packet Packet) (buffer.ByteBuf, error) {
	if alloc == nil || !packet.Valid() {
		return nil, ErrInvalidPacket
	}
	payloadSize := 0
	if packet.Payload != nil {
		payloadSize = packet.Payload.ReadableBytes()
	}
	header := packet.headerForEncode(payloadSize)
	headerBytes, err := AppendHeader(nil, header)
	if err != nil {
		return nil, err
	}
	out, err := alloc.Acquire(len(headerBytes) + payloadSize)
	if err != nil {
		return nil, err
	}
	if _, err := out.WriteBytes(headerBytes); err != nil {
		out.Release()
		return nil, err
	}
	if packet.Payload != nil && payloadSize > 0 {
		if _, err := out.WriteBytes(packet.Payload.Bytes()); err != nil {
			out.Release()
			return nil, err
		}
	}
	return out, nil
}

func (p Packet) headerForEncode(payloadSize int) Header {
	header := p.Header
	if header.Type == 0 {
		header.Type = p.Type
	}
	if header.Version == 0 {
		header.Version = p.Version
	}
	if header.DestinationID.Empty() {
		header.DestinationID = p.DestinationID
	}
	if header.SourceID.Empty() {
		header.SourceID = p.SourceID
	}
	if header.PacketNumberLength == 0 {
		header.PacketNumberLength = p.PacketNumberLength
	}
	if header.PacketNumberLength == 0 {
		header.PacketNumberLength = 1
	}
	if header.PacketNumber == 0 {
		header.PacketNumber = p.PacketNumber
	}
	if header.Type != PacketRetry && header.Type != PacketShort && header.Length == 0 {
		header.Length = uint64(header.PacketNumberLength + payloadSize)
	}
	return header
}

func asUDPAddressed(msg any) (udp.Addressed, bool) {
	switch v := msg.(type) {
	case udp.Addressed:
		return v, true
	case *udp.Addressed:
		if v == nil {
			return udp.Addressed{}, false
		}
		return *v, true
	default:
		return udp.Addressed{}, false
	}
}

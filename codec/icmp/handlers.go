package icmp

import (
	"encoding/binary"
	"net"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/raw"
)

type Decoder struct{}

func NewDecoder() *Decoder {
	return &Decoder{}
}

func (d *Decoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	addressed, ok := asRawAddressed(msg)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	payload, ok := addressed.Message.(buffer.ByteBuf)
	if !ok || !isSupportedProtocol(addressed.Protocol) {
		ctx.FireChannelRead(msg)
		return
	}
	decoded, err := Decode(payload, addressed.Protocol)
	addressed.Release()
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(raw.Addressed{Message: decoded, Addr: addressed.Addr, Protocol: addressed.Protocol})
}

type EncoderConfig struct {
	IPv6SourceIP net.IP
}

type Encoder struct {
	cfg EncoderConfig
}

func NewEncoder(cfg EncoderConfig) *Encoder {
	return &Encoder{cfg: cfg}
}

func (e *Encoder) Write(ctx *channel.HandlerContext, msg any) error {
	addressed, ok := asRawAddressed(msg)
	if !ok {
		return ctx.Write(msg)
	}
	message, ok := addressed.Message.(*Message)
	if !ok {
		return ctx.Write(msg)
	}
	protocol := addressed.Protocol
	if protocol == 0 {
		protocol = message.Protocol
	}
	if protocol == 0 {
		protocol = protocolForType(message.Type)
	}
	out, err := Encode(ctx.Channel().Allocator(), message, protocol, addressed.Addr.IP, e.cfg.IPv6SourceIP)
	message.Release()
	if err != nil {
		return err
	}
	if err := ctx.Write(raw.Addressed{Message: out, Addr: addressed.Addr, Protocol: protocol}); err != nil {
		out.Release()
		return err
	}
	return nil
}

func Decode(buf buffer.ByteBuf, protocol int) (*Message, error) {
	if !isSupportedProtocol(protocol) || buf == nil {
		return nil, ErrInvalidMessage
	}
	base, err := icmpBaseIndex(buf, protocol)
	if err != nil {
		return nil, err
	}
	if buf.WriterIndex()-base < messageHeaderLength {
		return nil, ErrInvalidMessage
	}
	t, ok := buf.GetByte(base)
	if !ok {
		return nil, ErrInvalidMessage
	}
	code, ok := buf.GetByte(base + 1)
	if !ok {
		return nil, ErrInvalidMessage
	}
	checksum, err := buf.ReadUnsigned(base+2, 2, buffer.BigEndian)
	if err != nil {
		return nil, err
	}
	msg := &Message{
		Type:     t,
		Code:     code,
		Checksum: uint16(checksum),
		Protocol: protocol,
	}
	payloadOffset := base + messageHeaderLength
	if isEchoType(t) {
		if buf.ReadableBytes() < echoHeaderLength {
			return nil, ErrInvalidMessage
		}
		id, err := buf.ReadUnsigned(base+4, 2, buffer.BigEndian)
		if err != nil {
			return nil, err
		}
		seq, err := buf.ReadUnsigned(base+6, 2, buffer.BigEndian)
		if err != nil {
			return nil, err
		}
		msg.Identifier = uint16(id)
		msg.Sequence = uint16(seq)
		payloadOffset = base + echoHeaderLength
	}
	if readable := buf.WriterIndex() - payloadOffset; readable > 0 {
		payload, err := buf.Slice(payloadOffset, readable)
		if err != nil {
			return nil, err
		}
		msg.Payload = payload
	}
	return msg, nil
}

func Encode(alloc buffer.Allocator, msg *Message, protocol int, dstIP net.IP, fallbackIPv6Source net.IP) (buffer.ByteBuf, error) {
	if alloc == nil || msg == nil || !isSupportedProtocol(protocol) {
		return nil, ErrInvalidMessage
	}
	size := msg.encodedLength()
	if size < messageHeaderLength {
		return nil, ErrInvalidMessage
	}
	out, err := alloc.Acquire(size)
	if err != nil {
		return nil, err
	}
	view := out.WritableBytesView()
	view[0] = msg.Type
	view[1] = msg.Code
	view[2] = 0
	view[3] = 0
	offset := messageHeaderLength
	if msg.IsEcho() {
		binary.BigEndian.PutUint16(view[4:6], msg.Identifier)
		binary.BigEndian.PutUint16(view[6:8], msg.Sequence)
		offset = echoHeaderLength
	}
	if msg.Payload != nil && msg.Payload.ReadableBytes() > 0 {
		copy(view[offset:], msg.Payload.Bytes())
	}
	if err := out.AdvanceWriter(size); err != nil {
		out.Release()
		return nil, err
	}
	bytes := out.Bytes()
	var checksum uint16
	if protocol == raw.ProtocolICMPv6 {
		source := msg.SourceIP
		if source == nil {
			source = fallbackIPv6Source
		}
		checksum, err = ChecksumIPv6(source, dstIP, bytes)
		if err != nil {
			out.Release()
			return nil, err
		}
	} else {
		checksum = Checksum(bytes)
	}
	binary.BigEndian.PutUint16(bytes[2:4], checksum)
	return out, nil
}

func isSupportedProtocol(protocol int) bool {
	return protocol == raw.ProtocolICMP || protocol == raw.ProtocolICMPv6
}

func isEchoType(t uint8) bool {
	return t == TypeEchoRequest || t == TypeEchoReply || t == TypeIPv6EchoRequest || t == TypeIPv6EchoReply
}

func icmpBaseIndex(buf buffer.ByteBuf, protocol int) (int, error) {
	base := buf.ReaderIndex()
	if protocol != raw.ProtocolICMP || buf.WriterIndex()-base < 20 {
		return base, nil
	}
	first, ok := buf.GetByte(base)
	if !ok || first>>4 != 4 {
		return base, nil
	}
	ihl := int(first&0x0f) * 4
	if ihl < 20 || buf.WriterIndex()-base < ihl+messageHeaderLength {
		return 0, ErrInvalidMessage
	}
	ipProtocol, ok := buf.GetByte(base + 9)
	if !ok || int(ipProtocol) != raw.ProtocolICMP {
		return 0, ErrInvalidMessage
	}
	return base + ihl, nil
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

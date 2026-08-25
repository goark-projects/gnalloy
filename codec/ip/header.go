package ip

import (
	"encoding/binary"
	"net"

	"goark.dev/gnalloy/buffer"
)

// ParseHeader 从 ByteBuf 的可读区解析 IPv4/IPv6 header。
func ParseHeader(buf buffer.ByteBuf) (Header, int, error) {
	if buf == nil || buf.ReadableBytes() < 1 {
		return Header{}, 0, ErrInvalidHeader
	}
	first, ok := buf.GetByte(buf.ReaderIndex())
	if !ok {
		return Header{}, 0, ErrInvalidHeader
	}
	switch int(first >> 4) {
	case Version4:
		return parseIPv4Header(buf)
	case Version6:
		return parseIPv6Header(buf)
	default:
		return Header{}, 0, ErrInvalidHeader
	}
}

// DecodePacket 解析完整 IP packet，payload 使用原 ByteBuf 的零拷贝切片。
func DecodePacket(buf buffer.ByteBuf) (Packet, error) {
	header, n, err := ParseHeader(buf)
	if err != nil {
		return Packet{}, err
	}
	payloadLength := header.payloadLength()
	if payloadLength < 0 || buf.WriterIndex()-buf.ReaderIndex() < n+payloadLength {
		return Packet{}, ErrInvalidPacket
	}
	packet := Packet{Header: header}
	if payloadLength > 0 {
		payload, err := buf.Slice(buf.ReaderIndex()+n, payloadLength)
		if err != nil {
			return Packet{}, err
		}
		packet.Payload = payload
	}
	return packet, nil
}

// EncodePacket 构建完整 IP packet。头部会重算长度与 IPv4 checksum。
func EncodePacket(alloc buffer.Allocator, packet Packet) (buffer.ByteBuf, error) {
	if alloc == nil || !packet.Valid() {
		return nil, ErrInvalidPacket
	}
	payloadLength := 0
	if packet.Payload != nil {
		payloadLength = packet.Payload.ReadableBytes()
	}
	headerBytes, err := AppendHeader(nil, packet.Header, payloadLength)
	if err != nil {
		return nil, err
	}
	out, err := alloc.Acquire(len(headerBytes) + payloadLength)
	if err != nil {
		return nil, err
	}
	if _, err := out.WriteBytes(headerBytes); err != nil {
		out.Release()
		return nil, err
	}
	if packet.Payload != nil && payloadLength > 0 {
		if _, err := out.WriteBytes(packet.Payload.Bytes()); err != nil {
			out.Release()
			return nil, err
		}
	}
	return out, nil
}

// AppendHeader 将 IPv4/IPv6 header 追加到 dst。
func AppendHeader(dst []byte, header Header, payloadLength int) ([]byte, error) {
	if payloadLength < 0 {
		return nil, ErrInvalidPacket
	}
	version := header.Version
	if version == 0 {
		version = inferVersion(header.Source, header.Destination)
	}
	switch version {
	case Version4:
		return appendIPv4Header(dst, header, payloadLength)
	case Version6:
		return appendIPv6Header(dst, header, payloadLength)
	default:
		return nil, ErrInvalidHeader
	}
}

func parseIPv4Header(buf buffer.ByteBuf) (Header, int, error) {
	base := buf.ReaderIndex()
	if buf.WriterIndex()-base < ipv4HeaderLength {
		return Header{}, 0, ErrInvalidHeader
	}
	first, _ := buf.GetByte(base)
	ihl := int(first&0x0f) * 4
	if ihl < ipv4HeaderLength || buf.WriterIndex()-base < ihl {
		return Header{}, 0, ErrInvalidHeader
	}
	total, err := buf.ReadUnsigned(base+2, 2, buffer.BigEndian)
	if err != nil {
		return Header{}, 0, err
	}
	totalLength := int(total)
	if totalLength < ihl || buf.WriterIndex()-base < totalLength {
		return Header{}, 0, ErrInvalidPacket
	}
	id, _ := buf.ReadUnsigned(base+4, 2, buffer.BigEndian)
	flagsFrag, _ := buf.ReadUnsigned(base+6, 2, buffer.BigEndian)
	ttl, _ := buf.GetByte(base + 8)
	proto, _ := buf.GetByte(base + 9)
	sum, _ := buf.ReadUnsigned(base+10, 2, buffer.BigEndian)
	headerBytes := make([]byte, ihl)
	copy(headerBytes, readableAt(buf, base, ihl))
	if Checksum(headerBytes) != 0 {
		return Header{}, 0, ErrInvalidHeader
	}
	src := append(net.IP(nil), readableAt(buf, base+12, 4)...)
	dst := append(net.IP(nil), readableAt(buf, base+16, 4)...)
	return Header{
		Version:        Version4,
		HeaderLength:   ihl,
		TotalLength:    totalLength,
		Identification: uint16(id),
		Flags:          uint8(flagsFrag >> 13),
		FragmentOffset: uint16(flagsFrag & 0x1fff),
		TTL:            ttl,
		Protocol:       int(proto),
		Checksum:       uint16(sum),
		Source:         src,
		Destination:    dst,
	}, ihl, nil
}

func parseIPv6Header(buf buffer.ByteBuf) (Header, int, error) {
	base := buf.ReaderIndex()
	if buf.WriterIndex()-base < ipv6HeaderLength {
		return Header{}, 0, ErrInvalidHeader
	}
	vtcfl, err := buf.ReadUnsigned(base, 4, buffer.BigEndian)
	if err != nil {
		return Header{}, 0, err
	}
	payloadLength, _ := buf.ReadUnsigned(base+4, 2, buffer.BigEndian)
	next, _ := buf.GetByte(base + 6)
	hop, _ := buf.GetByte(base + 7)
	if buf.WriterIndex()-base < ipv6HeaderLength+int(payloadLength) {
		return Header{}, 0, ErrInvalidPacket
	}
	src := append(net.IP(nil), readableAt(buf, base+8, 16)...)
	dst := append(net.IP(nil), readableAt(buf, base+24, 16)...)
	return Header{
		Version:       Version6,
		HeaderLength:  ipv6HeaderLength,
		TrafficClass:  int((vtcfl >> 20) & 0xff),
		FlowLabel:     uint32(vtcfl & 0xfffff),
		PayloadLength: int(payloadLength),
		NextHeader:    int(next),
		HopLimit:      hop,
		Source:        src,
		Destination:   dst,
	}, ipv6HeaderLength, nil
}

func appendIPv4Header(dst []byte, header Header, payloadLength int) ([]byte, error) {
	src := header.Source.To4()
	dstIP := header.Destination.To4()
	if src == nil || dstIP == nil {
		return nil, ErrInvalidAddress
	}
	protocol := header.Protocol
	if protocol == 0 {
		protocol = header.NextHeader
	}
	if !validProtocol(protocol) {
		return nil, ErrInvalidProtocol
	}
	if payloadLength > 0xffff-ipv4HeaderLength {
		return nil, ErrInvalidPacket
	}
	ttl := header.TTL
	if ttl == 0 {
		ttl = DefaultTTL
	}
	var tmp [ipv4HeaderLength]byte
	tmp[0] = byte(Version4<<4 | ipv4HeaderLength/4)
	tmp[1] = byte(header.TrafficClass)
	binary.BigEndian.PutUint16(tmp[2:4], uint16(ipv4HeaderLength+payloadLength))
	binary.BigEndian.PutUint16(tmp[4:6], header.Identification)
	flagsFrag := uint16(header.Flags&0x7)<<13 | (header.FragmentOffset & 0x1fff)
	binary.BigEndian.PutUint16(tmp[6:8], flagsFrag)
	tmp[8] = ttl
	tmp[9] = byte(protocol)
	copy(tmp[12:16], src)
	copy(tmp[16:20], dstIP)
	binary.BigEndian.PutUint16(tmp[10:12], Checksum(tmp[:]))
	return append(dst, tmp[:]...), nil
}

func appendIPv6Header(dst []byte, header Header, payloadLength int) ([]byte, error) {
	src := header.Source.To16()
	dstIP := header.Destination.To16()
	if src == nil || dstIP == nil || header.Source.To4() != nil || header.Destination.To4() != nil {
		return nil, ErrInvalidAddress
	}
	if payloadLength > 0xffff {
		return nil, ErrInvalidPacket
	}
	next := header.NextHeader
	if next == 0 {
		next = header.Protocol
	}
	if !validProtocol(next) {
		return nil, ErrInvalidProtocol
	}
	hop := header.HopLimit
	if hop == 0 {
		hop = DefaultHopLimit
	}
	var tmp [ipv6HeaderLength]byte
	vtcfl := uint32(Version6)<<28 | uint32(header.TrafficClass&0xff)<<20 | (header.FlowLabel & 0xfffff)
	binary.BigEndian.PutUint32(tmp[0:4], vtcfl)
	binary.BigEndian.PutUint16(tmp[4:6], uint16(payloadLength))
	tmp[6] = byte(next)
	tmp[7] = hop
	copy(tmp[8:24], src)
	copy(tmp[24:40], dstIP)
	return append(dst, tmp[:]...), nil
}

func readableAt(buf buffer.ByteBuf, index int, length int) []byte {
	return buf.Bytes()[index-buf.ReaderIndex() : index-buf.ReaderIndex()+length]
}

func inferVersion(src net.IP, dst net.IP) int {
	if src.To4() != nil && dst.To4() != nil {
		return Version4
	}
	if src.To16() != nil && dst.To16() != nil {
		return Version6
	}
	return 0
}

func validProtocol(protocol int) bool {
	return protocol > 0 && protocol <= 255
}

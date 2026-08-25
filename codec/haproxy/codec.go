package haproxy

import (
	"bytes"
	"net"
	"strconv"
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

type Decoder struct {
	*codec.ByteToMessageDecoder
	maxHeaderLength int
}

func NewDecoder(maxHeaderLength int) (*Decoder, error) {
	if maxHeaderLength <= 0 {
		maxHeaderLength = defaultMaxHeaderLength
	}
	if maxHeaderLength < maxV1HeaderLength {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &Decoder{maxHeaderLength: maxHeaderLength}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

func (d *Decoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if in.ReadableBytes() == 0 {
		return nil, nil
	}
	reader := in.ReaderIndex()
	v2State := prefixState(in, reader, v2Signature[:])
	if v2State == prefixFull {
		return d.decodeV2(in, reader)
	}
	v1State := prefixState(in, reader, []byte("PROXY "))
	switch {
	case v1State == prefixFull:
		return d.decodeV1(in, reader)
	case v2State == prefixPartial || v1State == prefixPartial:
		return nil, nil
	default:
		return nil, ErrInvalidFrame
	}
}

func (d *Decoder) decodeV1(in *buffer.CompositeByteBuf, reader int) (any, error) {
	lineEnd, ok := findCRLF(in, reader)
	if !ok {
		if in.ReadableBytes() > maxV1HeaderLength {
			return nil, codec.ErrFrameTooLong
		}
		return nil, nil
	}
	headerLen := lineEnd + 2 - reader
	if headerLen > maxV1HeaderLength || headerLen > d.maxHeaderLength {
		return nil, codec.ErrFrameTooLong
	}
	line, err := lineString(in, reader, lineEnd)
	if err != nil {
		return nil, err
	}
	msg, err := parseV1Line(line)
	if err != nil {
		return nil, err
	}
	if err := in.SkipBytes(headerLen); err != nil {
		return nil, err
	}
	return msg, nil
}

func (d *Decoder) decodeV2(in *buffer.CompositeByteBuf, reader int) (any, error) {
	if in.ReadableBytes() < 16 {
		return nil, nil
	}
	length, err := in.ReadUnsigned(reader+14, 2, buffer.BigEndian)
	if err != nil {
		return nil, err
	}
	total := 16 + int(length)
	if total > d.maxHeaderLength {
		return nil, codec.ErrFrameTooLong
	}
	if in.ReadableBytes() < total {
		return nil, nil
	}
	msg, err := parseV2Header(in, reader, int(length))
	if err != nil {
		return nil, err
	}
	if err := in.SkipBytes(total); err != nil {
		return nil, err
	}
	return msg, nil
}

type Encoder struct{}

func NewEncoder() *Encoder {
	return &Encoder{}
}

func (e *Encoder) Write(ctx *channel.HandlerContext, msg any) error {
	header, ok := msg.(Message)
	if !ok {
		if ptr, ptrOK := msg.(*Message); ptrOK && ptr != nil {
			header = *ptr
			ok = true
		}
	}
	if !ok {
		return ctx.Write(msg)
	}
	outBytes, err := AppendHeader(nil, header)
	if err != nil {
		return err
	}
	out, err := ctx.Channel().Allocator().Acquire(len(outBytes))
	if err != nil {
		return err
	}
	if _, err := out.WriteBytes(outBytes); err != nil {
		out.Release()
		return err
	}
	return ctx.Write(out)
}

func AppendHeader(dst []byte, msg Message) ([]byte, error) {
	if msg.Version == 0 {
		msg.Version = Version2
	}
	switch msg.Version {
	case Version1:
		return appendV1(dst, msg)
	case Version2:
		return appendV2(dst, msg)
	default:
		return nil, ErrInvalidFrame
	}
}

func parseV1Line(line string) (Message, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "PROXY" {
		return Message{}, ErrInvalidFrame
	}
	protocol, ok := protocolFromString(fields[1])
	if !ok {
		return Message{}, ErrUnsupportedProtocol
	}
	msg := Message{Version: Version1, Command: CommandProxy, Protocol: protocol}
	if protocol == ProtocolUnknown {
		return msg, nil
	}
	if len(fields) != 6 {
		return Message{}, ErrInvalidFrame
	}
	if net.ParseIP(fields[2]) == nil || net.ParseIP(fields[3]) == nil {
		return Message{}, ErrInvalidFrame
	}
	sourcePort, err := parsePort(fields[4])
	if err != nil {
		return Message{}, err
	}
	destPort, err := parsePort(fields[5])
	if err != nil {
		return Message{}, err
	}
	msg.SourceAddress = fields[2]
	msg.DestinationAddress = fields[3]
	msg.SourcePort = sourcePort
	msg.DestinationPort = destPort
	return msg, nil
}

func parseV2Header(in *buffer.CompositeByteBuf, reader int, payloadLength int) (Message, error) {
	versionCommand, _ := in.GetByte(reader + 12)
	if versionCommand>>4 != 0x02 {
		return Message{}, ErrInvalidFrame
	}
	command := Command(versionCommand & 0x0f)
	if command != CommandLocal && command != CommandProxy {
		return Message{}, ErrInvalidFrame
	}
	protocolByte, _ := in.GetByte(reader + 13)
	msg := Message{Version: Version2, Command: command, Protocol: Protocol(protocolByte)}
	payload := reader + 16
	if command == CommandLocal || msg.Protocol == ProtocolUnknown {
		return msg, nil
	}
	var tlvStart int
	switch msg.Protocol {
	case ProtocolTCP4, ProtocolUDP4:
		if payloadLength < 12 {
			return Message{}, ErrInvalidFrame
		}
		msg.SourceAddress = ipv4String(in, payload)
		msg.DestinationAddress = ipv4String(in, payload+4)
		sourcePort, err := in.ReadUnsigned(payload+8, 2, buffer.BigEndian)
		if err != nil {
			return Message{}, err
		}
		destPort, err := in.ReadUnsigned(payload+10, 2, buffer.BigEndian)
		if err != nil {
			return Message{}, err
		}
		msg.SourcePort = uint16(sourcePort)
		msg.DestinationPort = uint16(destPort)
		tlvStart = payload + 12
	case ProtocolTCP6, ProtocolUDP6:
		if payloadLength < 36 {
			return Message{}, ErrInvalidFrame
		}
		msg.SourceAddress = ipString(in, payload, net.IPv6len)
		msg.DestinationAddress = ipString(in, payload+16, net.IPv6len)
		sourcePort, err := in.ReadUnsigned(payload+32, 2, buffer.BigEndian)
		if err != nil {
			return Message{}, err
		}
		destPort, err := in.ReadUnsigned(payload+34, 2, buffer.BigEndian)
		if err != nil {
			return Message{}, err
		}
		msg.SourcePort = uint16(sourcePort)
		msg.DestinationPort = uint16(destPort)
		tlvStart = payload + 36
	case ProtocolUnixStream, ProtocolUnixDgram:
		if payloadLength < 216 {
			return Message{}, ErrInvalidFrame
		}
		msg.SourceAddress = unixAddressString(in, payload)
		msg.DestinationAddress = unixAddressString(in, payload+108)
		tlvStart = payload + 216
	default:
		return Message{}, ErrUnsupportedProtocol
	}
	tlvs, err := parseTLVs(in, tlvStart, reader+16+payloadLength)
	if err != nil {
		return Message{}, err
	}
	msg.TLVs = tlvs
	return msg, nil
}

func appendV1(dst []byte, msg Message) ([]byte, error) {
	if msg.Protocol == ProtocolUnknown {
		return append(dst, "PROXY UNKNOWN\r\n"...), nil
	}
	if msg.Protocol != ProtocolTCP4 && msg.Protocol != ProtocolTCP6 {
		return nil, ErrUnsupportedProtocol
	}
	if err := validateIPPair(msg); err != nil {
		return nil, err
	}
	dst = append(dst, "PROXY "...)
	dst = append(dst, msg.Protocol.String()...)
	dst = append(dst, ' ')
	dst = append(dst, msg.SourceAddress...)
	dst = append(dst, ' ')
	dst = append(dst, msg.DestinationAddress...)
	dst = append(dst, ' ')
	dst = strconv.AppendUint(dst, uint64(msg.SourcePort), 10)
	dst = append(dst, ' ')
	dst = strconv.AppendUint(dst, uint64(msg.DestinationPort), 10)
	dst = append(dst, "\r\n"...)
	if len(dst) > maxV1HeaderLength {
		return nil, codec.ErrFrameTooLong
	}
	return dst, nil
}

func appendV2(dst []byte, msg Message) ([]byte, error) {
	if msg.Command != CommandLocal && msg.Command != CommandProxy {
		if msg.Command == 0 && msg.Protocol != ProtocolUnknown {
			msg.Command = CommandProxy
		} else {
			return nil, ErrInvalidFrame
		}
	}
	addressLength, err := addressPayloadLength(msg)
	if err != nil {
		return nil, err
	}
	tlvLength, err := tlvPayloadLength(msg.TLVs)
	if err != nil {
		return nil, err
	}
	payloadLength := addressLength + tlvLength
	if payloadLength > 65535 {
		return nil, codec.ErrFrameTooLong
	}
	dst = append(dst, v2Signature[:]...)
	dst = append(dst, 0x20|byte(msg.Command), byte(msg.Protocol), byte(payloadLength>>8), byte(payloadLength))
	dst, err = appendAddressPayload(dst, msg)
	if err != nil {
		return nil, err
	}
	return appendTLVs(dst, msg.TLVs)
}

func addressPayloadLength(msg Message) (int, error) {
	if msg.Command == CommandLocal || msg.Protocol == ProtocolUnknown {
		return 0, nil
	}
	switch msg.Protocol {
	case ProtocolTCP4, ProtocolUDP4:
		return 12, validateIPPair(msg)
	case ProtocolTCP6, ProtocolUDP6:
		return 36, validateIPPair(msg)
	case ProtocolUnixStream, ProtocolUnixDgram:
		if len(msg.SourceAddress) > 108 || len(msg.DestinationAddress) > 108 {
			return 0, codec.ErrFrameTooLong
		}
		return 216, nil
	default:
		return 0, ErrUnsupportedProtocol
	}
}

func appendAddressPayload(dst []byte, msg Message) ([]byte, error) {
	if msg.Command == CommandLocal || msg.Protocol == ProtocolUnknown {
		return dst, nil
	}
	switch msg.Protocol {
	case ProtocolTCP4, ProtocolUDP4:
		src := net.ParseIP(msg.SourceAddress).To4()
		dstIP := net.ParseIP(msg.DestinationAddress).To4()
		if src == nil || dstIP == nil {
			return nil, ErrInvalidFrame
		}
		dst = append(dst, src...)
		dst = append(dst, dstIP...)
		return appendPorts(dst, msg.SourcePort, msg.DestinationPort), nil
	case ProtocolTCP6, ProtocolUDP6:
		src := net.ParseIP(msg.SourceAddress).To16()
		dstIP := net.ParseIP(msg.DestinationAddress).To16()
		if src == nil || dstIP == nil || net.ParseIP(msg.SourceAddress).To4() != nil || net.ParseIP(msg.DestinationAddress).To4() != nil {
			return nil, ErrInvalidFrame
		}
		dst = append(dst, src...)
		dst = append(dst, dstIP...)
		return appendPorts(dst, msg.SourcePort, msg.DestinationPort), nil
	case ProtocolUnixStream, ProtocolUnixDgram:
		dst = appendPadded(dst, msg.SourceAddress, 108)
		return appendPadded(dst, msg.DestinationAddress, 108), nil
	default:
		return nil, ErrUnsupportedProtocol
	}
}

func validateIPPair(msg Message) error {
	src := net.ParseIP(msg.SourceAddress)
	dst := net.ParseIP(msg.DestinationAddress)
	if src == nil || dst == nil {
		return ErrInvalidFrame
	}
	switch msg.Protocol {
	case ProtocolTCP4, ProtocolUDP4:
		if src.To4() == nil || dst.To4() == nil {
			return ErrInvalidFrame
		}
	case ProtocolTCP6, ProtocolUDP6:
		if src.To16() == nil || dst.To16() == nil || src.To4() != nil || dst.To4() != nil {
			return ErrInvalidFrame
		}
	}
	return nil
}

func appendPorts(dst []byte, source uint16, destination uint16) []byte {
	return append(dst, byte(source>>8), byte(source), byte(destination>>8), byte(destination))
}

func appendPadded(dst []byte, value string, size int) []byte {
	dst = append(dst, value...)
	for i := len(value); i < size; i++ {
		dst = append(dst, 0)
	}
	return dst
}

func tlvPayloadLength(tlvs []TLV) (int, error) {
	total := 0
	for _, tlv := range tlvs {
		if len(tlv.Value) > 65535 {
			return 0, codec.ErrFrameTooLong
		}
		total += 3 + len(tlv.Value)
	}
	return total, nil
}

func appendTLVs(dst []byte, tlvs []TLV) ([]byte, error) {
	for _, tlv := range tlvs {
		if len(tlv.Value) > 65535 {
			return nil, codec.ErrFrameTooLong
		}
		dst = append(dst, byte(tlv.Type), byte(len(tlv.Value)>>8), byte(len(tlv.Value)))
		dst = append(dst, tlv.Value...)
	}
	return dst, nil
}

func parseTLVs(in *buffer.CompositeByteBuf, start int, end int) ([]TLV, error) {
	var tlvs []TLV
	for start < end {
		if end-start < 3 {
			return nil, ErrInvalidTLV
		}
		tlvType, _ := in.GetByte(start)
		length, err := in.ReadUnsigned(start+1, 2, buffer.BigEndian)
		if err != nil {
			return nil, err
		}
		start += 3
		if int(length) > end-start {
			return nil, ErrInvalidTLV
		}
		value := make([]byte, int(length))
		for i := range value {
			value[i], _ = in.GetByte(start + i)
		}
		tlvs = append(tlvs, TLV{Type: TLVType(tlvType), Value: value})
		start += int(length)
	}
	return tlvs, nil
}

func parsePort(value string) (uint16, error) {
	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, ErrInvalidFrame
	}
	return uint16(port), nil
}

type prefixMatch uint8

const (
	prefixMismatch prefixMatch = iota
	prefixPartial
	prefixFull
)

func prefixState(in *buffer.CompositeByteBuf, start int, prefix []byte) prefixMatch {
	readable := in.WriterIndex() - start
	limit := len(prefix)
	if readable < limit {
		limit = readable
	}
	for i := 0; i < limit; i++ {
		b, ok := in.GetByte(start + i)
		if !ok || b != prefix[i] {
			return prefixMismatch
		}
	}
	if readable < len(prefix) {
		return prefixPartial
	}
	return prefixFull
}

func findCRLF(in *buffer.CompositeByteBuf, start int) (int, bool) {
	for i := start; i+1 < in.WriterIndex(); i++ {
		a, ok := in.GetByte(i)
		if !ok || a != '\r' {
			continue
		}
		b, ok := in.GetByte(i + 1)
		if ok && b == '\n' {
			return i, true
		}
	}
	return 0, false
}

func lineString(in *buffer.CompositeByteBuf, start int, end int) (string, error) {
	part, err := in.Slice(start, end-start)
	if err != nil {
		return "", err
	}
	defer part.Release()
	return string(part.Bytes()), nil
}

func ipv4String(in *buffer.CompositeByteBuf, start int) string {
	var ip [4]byte
	for i := range ip {
		ip[i], _ = in.GetByte(start + i)
	}
	return net.IP(ip[:]).String()
}

func ipString(in *buffer.CompositeByteBuf, start int, length int) string {
	ip := make(net.IP, length)
	for i := range ip {
		ip[i], _ = in.GetByte(start + i)
	}
	return ip.String()
}

func unixAddressString(in *buffer.CompositeByteBuf, start int) string {
	value := make([]byte, 108)
	for i := range value {
		value[i], _ = in.GetByte(start + i)
	}
	return string(bytes.TrimRight(value, "\x00"))
}

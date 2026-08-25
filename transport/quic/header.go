package quic

import (
	"encoding/binary"

	"goark.dev/gnalloy/buffer"
)

const (
	headerFormBit        = 0x80
	fixedBit             = 0x40
	longPacketTypeMask   = 0x30
	longPacketTypeShift  = 4
	packetNumberLenMask  = 0x03
	shortHeaderFlagsMask = 0x3c
	longHeaderFlagsMask  = 0x0c
	retryFlagsMask       = 0x0f
	maxVarInt            = 1<<62 - 1
)

// HeaderForm 描述 QUIC 包头使用长包头还是短包头。
type HeaderForm uint8

const (
	HeaderFormShort HeaderForm = iota + 1
	HeaderFormLong
)

// HeaderParseOptions 保存解析短包头所需的连接上下文。
type HeaderParseOptions struct {
	// ShortDestinationIDLength 是短包头 DCID 长度，QUIC 短包头本身不携带该字段。
	ShortDestinationIDLength int
}

// Header 是 QUIC packet header 的结构化表示，不包含 payload。
type Header struct {
	Form HeaderForm
	Type PacketType

	Version       Version
	DestinationID ConnectionID
	SourceID      ConnectionID

	// Flags 保留 first byte 中由 header protection 覆盖的标志位。
	Flags uint8

	PacketNumberLength int
	PacketNumber       uint64

	// Initial 构建时使用 Token；解析时只记录 TokenLength，避免复制 token 字节。
	Token       []byte
	TokenLength uint64

	// Length 是长包头中的 length 字段，包含 packet number 和 payload 长度。
	Length uint64

	// HeaderLength 是解析后包头占用的字节数。
	HeaderLength int
}

// ParseHeader 从 ByteBuf 的可读区域解析 QUIC header。
func ParseHeader(buf buffer.ByteBuf, opts HeaderParseOptions) (Header, int, error) {
	if buf == nil {
		return Header{}, 0, ErrInvalidHeader
	}
	return ParseHeaderBytes(buf.Bytes(), opts)
}

// ParseHeaderBytes 从连续字节视图解析 QUIC header。
func ParseHeaderBytes(data []byte, opts HeaderParseOptions) (Header, int, error) {
	if len(data) < 1 {
		return Header{}, 0, ErrInvalidHeader
	}
	first := data[0]
	if first&fixedBit == 0 {
		return Header{}, 0, ErrInvalidHeader
	}
	if first&headerFormBit != 0 {
		return parseLongHeader(data, first)
	}
	return parseShortHeader(data, first, opts.ShortDestinationIDLength)
}

// AppendHeader 把 Header 编码追加到 dst，返回追加后的切片。
func AppendHeader(dst []byte, h Header) ([]byte, error) {
	form := h.Form
	if form == 0 {
		if h.Type == PacketShort {
			form = HeaderFormShort
		} else {
			form = HeaderFormLong
		}
	}
	switch form {
	case HeaderFormLong:
		return appendLongHeader(dst, h)
	case HeaderFormShort:
		return appendShortHeader(dst, h)
	default:
		return nil, ErrInvalidHeader
	}
}

// ParseVarInt 解析 QUIC variable-length integer。
func ParseVarInt(data []byte) (uint64, int, error) {
	if len(data) == 0 {
		return 0, 0, ErrInvalidVarInt
	}
	prefix := data[0] >> 6
	n := 1 << prefix
	if len(data) < int(n) {
		return 0, 0, ErrInvalidVarInt
	}
	switch n {
	case 1:
		return uint64(data[0] & 0x3f), 1, nil
	case 2:
		return uint64(binary.BigEndian.Uint16(data[:2]) & 0x3fff), 2, nil
	case 4:
		return uint64(binary.BigEndian.Uint32(data[:4]) & 0x3fffffff), 4, nil
	case 8:
		return binary.BigEndian.Uint64(data[:8]) & maxVarInt, 8, nil
	default:
		return 0, 0, ErrInvalidVarInt
	}
}

// AppendVarInt 使用最短编码追加 QUIC variable-length integer。
func AppendVarInt(dst []byte, v uint64) ([]byte, error) {
	switch {
	case v <= 63:
		return append(dst, byte(v)), nil
	case v <= 16383:
		var tmp [2]byte
		binary.BigEndian.PutUint16(tmp[:], uint16(v)|0x4000)
		return append(dst, tmp[:]...), nil
	case v <= 1073741823:
		var tmp [4]byte
		binary.BigEndian.PutUint32(tmp[:], uint32(v)|0x80000000)
		return append(dst, tmp[:]...), nil
	case v <= maxVarInt:
		var tmp [8]byte
		binary.BigEndian.PutUint64(tmp[:], v|0xc000000000000000)
		return append(dst, tmp[:]...), nil
	default:
		return nil, ErrInvalidVarInt
	}
}

func parseLongHeader(data []byte, first byte) (Header, int, error) {
	if len(data) < 7 {
		return Header{}, 0, ErrInvalidHeader
	}
	packetType, err := packetTypeFromLongBits((first & longPacketTypeMask) >> longPacketTypeShift)
	if err != nil {
		return Header{}, 0, err
	}
	idx := 1
	version := Version(binary.BigEndian.Uint32(data[idx : idx+4]))
	if !version.Valid() {
		return Header{}, 0, ErrInvalidVersion
	}
	idx += 4
	dcid, n, err := parseConnectionID(data[idx:])
	if err != nil {
		return Header{}, 0, err
	}
	idx += n
	scid, n, err := parseConnectionID(data[idx:])
	if err != nil {
		return Header{}, 0, err
	}
	idx += n

	h := Header{
		Form:               HeaderFormLong,
		Type:               packetType,
		Version:            version,
		DestinationID:      dcid,
		SourceID:           scid,
		PacketNumberLength: int(first&packetNumberLenMask) + 1,
	}
	if packetType == PacketRetry {
		h.Flags = first & retryFlagsMask
		h.TokenLength = uint64(len(data) - idx)
		h.HeaderLength = len(data)
		return h, h.HeaderLength, nil
	}
	h.Flags = first & longHeaderFlagsMask
	if packetType == PacketInitial {
		tokenLength, consumed, err := ParseVarInt(data[idx:])
		if err != nil {
			return Header{}, 0, err
		}
		idx += consumed
		if tokenLength > uint64(len(data)-idx) {
			return Header{}, 0, ErrInvalidHeader
		}
		h.TokenLength = tokenLength
		idx += int(tokenLength)
	}
	length, consumed, err := ParseVarInt(data[idx:])
	if err != nil {
		return Header{}, 0, err
	}
	h.Length = length
	idx += consumed
	if idx+h.PacketNumberLength > len(data) {
		return Header{}, 0, ErrInvalidHeader
	}
	h.PacketNumber = decodePacketNumber(data[idx : idx+h.PacketNumberLength])
	idx += h.PacketNumberLength
	h.HeaderLength = idx
	return h, idx, nil
}

func parseShortHeader(data []byte, first byte, dcidLength int) (Header, int, error) {
	if dcidLength <= 0 || dcidLength > MaxConnectionIDLength {
		return Header{}, 0, ErrInvalidConnectionID
	}
	pnLen := int(first&packetNumberLenMask) + 1
	if len(data) < 1+dcidLength+pnLen {
		return Header{}, 0, ErrInvalidHeader
	}
	dcid, err := NewConnectionID(data[1 : 1+dcidLength])
	if err != nil {
		return Header{}, 0, err
	}
	pnStart := 1 + dcidLength
	h := Header{
		Form:               HeaderFormShort,
		Type:               PacketShort,
		DestinationID:      dcid,
		Flags:              first & shortHeaderFlagsMask,
		PacketNumberLength: pnLen,
		PacketNumber:       decodePacketNumber(data[pnStart : pnStart+pnLen]),
		HeaderLength:       pnStart + pnLen,
	}
	return h, h.HeaderLength, nil
}

func appendLongHeader(dst []byte, h Header) ([]byte, error) {
	if h.Type == PacketShort || h.Type == 0 || h.Type > PacketRetry {
		return nil, ErrInvalidHeader
	}
	if !h.Version.Valid() {
		return nil, ErrInvalidVersion
	}
	typeBits, err := longBitsFromPacketType(h.Type)
	if err != nil {
		return nil, err
	}
	pnLen := h.PacketNumberLength
	if h.Type == PacketRetry {
		first := byte(headerFormBit | fixedBit | typeBits<<longPacketTypeShift | (h.Flags & retryFlagsMask))
		dst = append(dst, first)
		dst = appendVersionAndConnectionIDs(dst, h)
		dst = append(dst, h.Token...)
		return dst, nil
	}
	if !validPacketNumberLength(pnLen) || !packetNumberFits(h.PacketNumber, pnLen) {
		return nil, ErrInvalidHeader
	}
	first := byte(headerFormBit | fixedBit | typeBits<<longPacketTypeShift | (h.Flags & longHeaderFlagsMask) | byte(pnLen-1))
	dst = append(dst, first)
	dst = appendVersionAndConnectionIDs(dst, h)
	if h.Type == PacketInitial {
		var err error
		dst, err = AppendVarInt(dst, uint64(len(h.Token)))
		if err != nil {
			return nil, err
		}
		dst = append(dst, h.Token...)
	}
	length := h.Length
	if length == 0 {
		length = uint64(pnLen)
	}
	if length < uint64(pnLen) {
		return nil, ErrInvalidHeader
	}
	dst, err = AppendVarInt(dst, length)
	if err != nil {
		return nil, err
	}
	return appendPacketNumber(dst, h.PacketNumber, pnLen), nil
}

func appendShortHeader(dst []byte, h Header) ([]byte, error) {
	if h.Type != 0 && h.Type != PacketShort {
		return nil, ErrInvalidHeader
	}
	pnLen := h.PacketNumberLength
	if !validPacketNumberLength(pnLen) || !packetNumberFits(h.PacketNumber, pnLen) || h.DestinationID.Empty() {
		return nil, ErrInvalidHeader
	}
	first := byte(fixedBit | (h.Flags & shortHeaderFlagsMask) | byte(pnLen-1))
	dst = append(dst, first)
	dst = h.DestinationID.AppendTo(dst)
	return appendPacketNumber(dst, h.PacketNumber, pnLen), nil
}

func appendVersionAndConnectionIDs(dst []byte, h Header) []byte {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], uint32(h.Version))
	dst = append(dst, tmp[:]...)
	dst = append(dst, byte(h.DestinationID.Len()))
	dst = h.DestinationID.AppendTo(dst)
	dst = append(dst, byte(h.SourceID.Len()))
	dst = h.SourceID.AppendTo(dst)
	return dst
}

func parseConnectionID(data []byte) (ConnectionID, int, error) {
	if len(data) < 1 {
		return ConnectionID{}, 0, ErrInvalidHeader
	}
	n := int(data[0])
	if n > MaxConnectionIDLength || len(data) < 1+n {
		return ConnectionID{}, 0, ErrInvalidConnectionID
	}
	cid, err := NewConnectionID(data[1 : 1+n])
	if err != nil {
		return ConnectionID{}, 0, err
	}
	return cid, 1 + n, nil
}

func packetTypeFromLongBits(bits byte) (PacketType, error) {
	switch bits {
	case 0:
		return PacketInitial, nil
	case 1:
		return PacketZeroRTT, nil
	case 2:
		return PacketHandshake, nil
	case 3:
		return PacketRetry, nil
	default:
		return 0, ErrInvalidHeader
	}
}

func longBitsFromPacketType(t PacketType) (byte, error) {
	switch t {
	case PacketInitial:
		return 0, nil
	case PacketZeroRTT:
		return 1, nil
	case PacketHandshake:
		return 2, nil
	case PacketRetry:
		return 3, nil
	default:
		return 0, ErrInvalidHeader
	}
}

func validPacketNumberLength(n int) bool {
	return n >= 1 && n <= 4
}

func packetNumberFits(v uint64, n int) bool {
	return n == 4 || v < 1<<uint(n*8)
}

func appendPacketNumber(dst []byte, v uint64, n int) []byte {
	for i := n - 1; i >= 0; i-- {
		dst = append(dst, byte(v>>uint(i*8)))
	}
	return dst
}

func decodePacketNumber(data []byte) uint64 {
	var v uint64
	for _, b := range data {
		v = (v << 8) | uint64(b)
	}
	return v
}

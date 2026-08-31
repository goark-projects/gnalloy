package http3

import (
	"goark.dev/gnalloy/buffer"

	"golang.org/x/net/http2/hpack"
)

const (
	qpackHuffmanMask7 = 0x80
	qpackHuffmanMask5 = 0x20
)

// QPACKSetDynamicTableCapacity 表示 encoder stream 的 Set Dynamic Table Capacity 指令。
type QPACKSetDynamicTableCapacity struct {
	Capacity uint64
}

// QPACKInsertWithNameRef 表示 encoder stream 的 Insert With Name Reference 指令。
type QPACKInsertWithNameRef struct {
	Static    bool
	NameIndex uint64
	Value     string
}

// QPACKInsertWithoutNameRef 表示 encoder stream 的 Insert Without Name Reference 指令。
type QPACKInsertWithoutNameRef struct {
	Field HeaderField
}

// QPACKDuplicate 表示 encoder stream 的 Duplicate 指令。
type QPACKDuplicate struct {
	Index uint64
}

// QPACKSectionAcknowledgment 表示 decoder stream 的 Section Acknowledgment 指令。
type QPACKSectionAcknowledgment struct {
	StreamID uint64
}

// QPACKStreamCancellation 表示 decoder stream 的 Stream Cancellation 指令。
type QPACKStreamCancellation struct {
	StreamID uint64
}

// QPACKInsertCountIncrement 表示 decoder stream 的 Insert Count Increment 指令。
type QPACKInsertCountIncrement struct {
	Increment uint64
}

func appendQPACKEncoderInstruction(dst []byte, msg any) ([]byte, bool, error) {
	switch inst := msg.(type) {
	case QPACKSetDynamicTableCapacity:
		out, err := appendQPACKPrefixedInt(dst, 5, 0x20, inst.Capacity)
		return out, true, err
	case QPACKInsertWithNameRef:
		pattern := byte(0x80)
		if inst.Static {
			pattern |= 0x40
		}
		out, err := appendQPACKPrefixedInt(dst, 6, pattern, inst.NameIndex)
		if err != nil {
			return nil, true, err
		}
		out, err = appendQPACKString(out, 7, 0, inst.Value)
		return out, true, err
	case QPACKInsertWithoutNameRef:
		out, err := appendQPACKString(dst, 5, 0x40, inst.Field.Name)
		if err != nil {
			return nil, true, err
		}
		out, err = appendQPACKString(out, 7, 0, inst.Field.Value)
		return out, true, err
	case QPACKDuplicate:
		out, err := appendQPACKPrefixedInt(dst, 5, 0, inst.Index)
		return out, true, err
	default:
		return dst, false, nil
	}
}

func decodeQPACKEncoderInstruction(in *buffer.CompositeByteBuf, index int) (any, int, bool, error) {
	first, ok := in.GetByte(index)
	if !ok {
		return nil, 0, false, nil
	}
	switch {
	case first&0x80 != 0:
		nameIndex, n, ok, err := readQPACKPrefixedInt(in, index, 6)
		if err != nil || !ok {
			return nil, 0, ok, err
		}
		value, valueLen, ok, err := readQPACKString(in, index+n, 7, qpackHuffmanMask7)
		if err != nil || !ok {
			return nil, 0, ok, err
		}
		return QPACKInsertWithNameRef{Static: first&0x40 != 0, NameIndex: nameIndex, Value: value}, n + valueLen, true, nil
	case first&0xe0 == 0x20:
		capacity, n, ok, err := readQPACKPrefixedInt(in, index, 5)
		if err != nil || !ok {
			return nil, 0, ok, err
		}
		return QPACKSetDynamicTableCapacity{Capacity: capacity}, n, true, nil
	case first&0xc0 == 0x40:
		name, n, ok, err := readQPACKString(in, index, 5, qpackHuffmanMask5)
		if err != nil || !ok {
			return nil, 0, ok, err
		}
		value, valueLen, ok, err := readQPACKString(in, index+n, 7, qpackHuffmanMask7)
		if err != nil || !ok {
			return nil, 0, ok, err
		}
		return QPACKInsertWithoutNameRef{Field: HeaderField{Name: name, Value: value}}, n + valueLen, true, nil
	case first&0xe0 == 0:
		index, n, ok, err := readQPACKPrefixedInt(in, index, 5)
		if err != nil || !ok {
			return nil, 0, ok, err
		}
		return QPACKDuplicate{Index: index}, n, true, nil
	default:
		return nil, 0, false, ErrQPACKInvalidInstruction
	}
}

func appendQPACKDecoderInstruction(dst []byte, msg any) ([]byte, bool, error) {
	switch inst := msg.(type) {
	case QPACKSectionAcknowledgment:
		out, err := appendQPACKPrefixedInt(dst, 7, 0x80, inst.StreamID)
		return out, true, err
	case QPACKStreamCancellation:
		out, err := appendQPACKPrefixedInt(dst, 6, 0x40, inst.StreamID)
		return out, true, err
	case QPACKInsertCountIncrement:
		if inst.Increment == 0 {
			return nil, true, ErrQPACKInvalidInstruction
		}
		out, err := appendQPACKPrefixedInt(dst, 6, 0, inst.Increment)
		return out, true, err
	default:
		return dst, false, nil
	}
}

func decodeQPACKDecoderInstruction(in *buffer.CompositeByteBuf, index int) (any, int, bool, error) {
	first, ok := in.GetByte(index)
	if !ok {
		return nil, 0, false, nil
	}
	switch {
	case first&0x80 != 0:
		streamID, n, ok, err := readQPACKPrefixedInt(in, index, 7)
		if err != nil || !ok {
			return nil, 0, ok, err
		}
		return QPACKSectionAcknowledgment{StreamID: streamID}, n, true, nil
	case first&0xc0 == 0x40:
		streamID, n, ok, err := readQPACKPrefixedInt(in, index, 6)
		if err != nil || !ok {
			return nil, 0, ok, err
		}
		return QPACKStreamCancellation{StreamID: streamID}, n, true, nil
	case first&0xc0 == 0:
		increment, n, ok, err := readQPACKPrefixedInt(in, index, 6)
		if err != nil || !ok {
			return nil, 0, ok, err
		}
		if increment == 0 {
			return nil, 0, false, ErrQPACKInvalidInstruction
		}
		return QPACKInsertCountIncrement{Increment: increment}, n, true, nil
	default:
		return nil, 0, false, ErrQPACKInvalidInstruction
	}
}

func appendQPACKString(dst []byte, prefixBits uint8, pattern byte, value string) ([]byte, error) {
	if uint64(len(value)) > maxVarInt {
		return nil, ErrInvalidVarInt
	}
	out, err := appendQPACKPrefixedInt(dst, prefixBits, pattern, uint64(len(value)))
	if err != nil {
		return nil, err
	}
	return append(out, value...), nil
}

func readQPACKString(in *buffer.CompositeByteBuf, index int, prefixBits uint8, huffmanMask byte) (string, int, bool, error) {
	length, n, ok, err := readQPACKPrefixedInt(in, index, prefixBits)
	if err != nil || !ok {
		return "", 0, ok, err
	}
	if length > uint64(in.WriterIndex()-index-n) {
		return "", 0, false, nil
	}
	if length > uint64(int(^uint(0)>>1)) {
		return "", 0, false, ErrInvalidVarInt
	}
	raw := make([]byte, int(length))
	for i := range raw {
		b, ok := in.GetByte(index + n + i)
		if !ok {
			return "", 0, false, nil
		}
		raw[i] = b
	}
	first, _ := in.GetByte(index)
	if first&huffmanMask == 0 {
		return string(raw), n + int(length), true, nil
	}
	value, err := hpack.HuffmanDecodeToString(raw)
	if err != nil {
		return "", 0, false, err
	}
	return value, n + int(length), true, nil
}

func appendQPACKPrefixedInt(dst []byte, prefixBits uint8, pattern byte, value uint64) ([]byte, error) {
	if prefixBits == 0 || prefixBits > 8 || value > maxVarInt {
		return nil, ErrInvalidVarInt
	}
	mask := uint64((1 << prefixBits) - 1)
	if value < mask {
		return append(dst, pattern|byte(value)), nil
	}
	dst = append(dst, pattern|byte(mask))
	value -= mask
	for value >= 128 {
		dst = append(dst, byte(value&0x7f)|0x80)
		value >>= 7
	}
	return append(dst, byte(value)), nil
}

func readQPACKPrefixedInt(in *buffer.CompositeByteBuf, index int, prefixBits uint8) (uint64, int, bool, error) {
	if prefixBits == 0 || prefixBits > 8 {
		return 0, 0, false, ErrInvalidVarInt
	}
	first, ok := in.GetByte(index)
	if !ok {
		return 0, 0, false, nil
	}
	mask := uint64((1 << prefixBits) - 1)
	value := uint64(first) & mask
	if value < mask {
		return value, 1, true, nil
	}
	value = mask
	shift := uint(0)
	for n := 1; ; n++ {
		b, ok := in.GetByte(index + n)
		if !ok {
			return 0, 0, false, nil
		}
		if shift >= 63 {
			return 0, 0, false, ErrInvalidVarInt
		}
		value += uint64(b&0x7f) << shift
		if value > maxVarInt {
			return 0, 0, false, ErrInvalidVarInt
		}
		if b&0x80 == 0 {
			return value, n + 1, true, nil
		}
		shift += 7
	}
}

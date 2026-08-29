package websocket

import (
	"encoding/binary"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/codec"
)

type frameHeader struct {
	first         byte
	second        byte
	payloadLength int
	headerLength  int
}

func decodeBasicFrameHeader(in *buffer.CompositeByteBuf, reader int) (frameHeader, bool) {
	prefix, ok := readableFramePrefix(in, reader, 2)
	if !ok {
		return frameHeader{}, false
	}
	return frameHeader{
		first:         prefix[0],
		second:        prefix[1],
		payloadLength: int(prefix[1] & 0x7f),
		headerLength:  2,
	}, true
}

func completeFrameHeader(in *buffer.CompositeByteBuf, reader int, header frameHeader) (frameHeader, bool, error) {
	switch header.payloadLength {
	case 126:
		extended, ok := readableFramePrefix(in, reader, 4)
		if !ok {
			return frameHeader{}, false, nil
		}
		header.payloadLength = int(binary.BigEndian.Uint16(extended[2:4]))
		header.headerLength = 4
	case 127:
		extended, ok := readableFramePrefix(in, reader, 10)
		if !ok {
			return frameHeader{}, false, nil
		}
		n := binary.BigEndian.Uint64(extended[2:10])
		if n > uint64(^uint(0)>>1) {
			return frameHeader{}, false, codec.ErrFrameTooLong
		}
		header.payloadLength = int(n)
		header.headerLength = 10
	}
	if header.second&0x80 != 0 {
		header.headerLength += 4
	}
	return header, true, nil
}

func readableFramePrefix(in *buffer.CompositeByteBuf, reader int, length int) ([]byte, bool) {
	if in.ReadableBytes() < length {
		return nil, false
	}
	if span, ok := in.ReadableSpan(reader, length); ok {
		return span, true
	}
	tmp, ok := readFramePrefixSlow(in, reader, length)
	if !ok {
		return nil, false
	}
	return tmp[:length], true
}

func readMaskKey(in *buffer.CompositeByteBuf, reader int, headerLength int) [4]byte {
	var mask [4]byte
	start := reader + headerLength - len(mask)
	if span, ok := in.ReadableSpan(start, len(mask)); ok {
		copy(mask[:], span)
		return mask
	}
	for i := range mask {
		mask[i], _ = in.GetByte(start + i)
	}
	return mask
}

func readFramePrefixSlow(in *buffer.CompositeByteBuf, reader int, length int) ([10]byte, bool) {
	var tmp [10]byte
	for i := 0; i < length; i++ {
		b, ok := in.GetByte(reader + i)
		if !ok {
			return [10]byte{}, false
		}
		tmp[i] = b
	}
	return tmp, true
}

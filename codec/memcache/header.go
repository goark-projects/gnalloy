package memcache

import (
	"encoding/binary"

	"goark.dev/gnalloy/buffer"
)

type binaryHeader struct {
	magic           byte
	opcode          byte
	keyLength       uint16
	extrasLength    byte
	dataType        byte
	vbucketOrStatus uint16
	bodyLength      uint32
	opaque          uint32
	cas             uint64
}

func decodeBinaryHeader(in *buffer.CompositeByteBuf, reader int) (binaryHeader, error) {
	if header, ok := contiguousHeader(in, reader); ok {
		return binaryHeader{
			magic:           header[0],
			opcode:          header[1],
			keyLength:       binary.BigEndian.Uint16(header[2:4]),
			extrasLength:    header[4],
			dataType:        header[5],
			vbucketOrStatus: binary.BigEndian.Uint16(header[6:8]),
			bodyLength:      binary.BigEndian.Uint32(header[8:12]),
			opaque:          binary.BigEndian.Uint32(header[12:16]),
			cas:             binary.BigEndian.Uint64(header[16:24]),
		}, nil
	}
	return decodeBinaryHeaderSlow(in, reader)
}

func contiguousHeader(in *buffer.CompositeByteBuf, reader int) ([]byte, bool) {
	return in.ReadableSpan(reader, HeaderLength)
}

func decodeBinaryHeaderSlow(in *buffer.CompositeByteBuf, reader int) (binaryHeader, error) {
	magic, ok := in.GetByte(reader)
	if !ok {
		return binaryHeader{}, buffer.ErrInvalidIndex
	}
	opcode, ok := in.GetByte(reader + 1)
	if !ok {
		return binaryHeader{}, buffer.ErrInvalidIndex
	}
	keyLength, err := in.ReadUnsigned(reader+2, 2, buffer.BigEndian)
	if err != nil {
		return binaryHeader{}, err
	}
	extrasLength, ok := in.GetByte(reader + 4)
	if !ok {
		return binaryHeader{}, buffer.ErrInvalidIndex
	}
	dataType, ok := in.GetByte(reader + 5)
	if !ok {
		return binaryHeader{}, buffer.ErrInvalidIndex
	}
	vbucketOrStatus, err := in.ReadUnsigned(reader+6, 2, buffer.BigEndian)
	if err != nil {
		return binaryHeader{}, err
	}
	bodyLength, err := in.ReadUnsigned(reader+8, 4, buffer.BigEndian)
	if err != nil {
		return binaryHeader{}, err
	}
	opaque, err := in.ReadUnsigned(reader+12, 4, buffer.BigEndian)
	if err != nil {
		return binaryHeader{}, err
	}
	cas, err := in.ReadUnsigned(reader+16, 8, buffer.BigEndian)
	if err != nil {
		return binaryHeader{}, err
	}
	return binaryHeader{
		magic:           magic,
		opcode:          opcode,
		keyLength:       uint16(keyLength),
		extrasLength:    extrasLength,
		dataType:        dataType,
		vbucketOrStatus: uint16(vbucketOrStatus),
		bodyLength:      uint32(bodyLength),
		opaque:          uint32(opaque),
		cas:             cas,
	}, nil
}

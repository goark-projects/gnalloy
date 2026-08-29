package spdy

import (
	"goark.dev/gnalloy/buffer"
)

type frameHeader struct {
	control bool
	version uint16
	kind    FrameType
	flags   byte
	length  int
	stream  uint32
}

func decodeFrameHeader(in *buffer.CompositeByteBuf, reader int) (frameHeader, error) {
	if header, ok := in.ReadableSpan(reader, headerSize); ok {
		return frameHeaderFromBytes(header), nil
	}
	return decodeFrameHeaderSlow(in, reader)
}

func frameHeaderFromBytes(header []byte) frameHeader {
	control := header[0]&0x80 != 0
	length := int(header[5])<<16 | int(header[6])<<8 | int(header[7])
	if control {
		return frameHeader{
			control: true,
			version: uint16(header[0]&0x7f)<<8 |
				uint16(header[1]),
			kind:   FrameType(uint16(header[2])<<8 | uint16(header[3])),
			flags:  header[4],
			length: length,
		}
	}
	return frameHeader{
		flags:  header[4],
		length: length,
		stream: uint32(header[0]&0x7f)<<24 |
			uint32(header[1])<<16 |
			uint32(header[2])<<8 |
			uint32(header[3]),
	}
}

func decodeFrameHeaderSlow(in *buffer.CompositeByteBuf, reader int) (frameHeader, error) {
	first, _ := in.GetByte(reader)
	control := first&0x80 != 0
	flags, _ := in.GetByte(reader + 4)
	length, err := readMedium(in, reader+5)
	if err != nil {
		return frameHeader{}, err
	}
	if control {
		versionAndControl, err := in.ReadUnsigned(reader, 2, buffer.BigEndian)
		if err != nil {
			return frameHeader{}, err
		}
		rawType, err := in.ReadUnsigned(reader+2, 2, buffer.BigEndian)
		if err != nil {
			return frameHeader{}, err
		}
		return frameHeader{
			control: true,
			version: uint16(versionAndControl & 0x7fff),
			kind:    FrameType(rawType),
			flags:   flags,
			length:  int(length),
		}, nil
	}
	streamID, err := readStreamID(in, reader)
	if err != nil {
		return frameHeader{}, err
	}
	return frameHeader{flags: flags, length: int(length), stream: streamID}, nil
}

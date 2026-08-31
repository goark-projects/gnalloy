package ascii

import (
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

// ResponseDecoder 解码 Memcached ASCII response。
type ResponseDecoder struct {
	*codec.ByteToMessageDecoder
	maxLineBytes  int
	maxValueBytes int
	values        []Value
}

// NewResponseDecoder 创建 response decoder。
func NewResponseDecoder(maxLineBytes int, maxValueBytes int) (*ResponseDecoder, error) {
	if maxLineBytes <= 0 || maxValueBytes < 0 {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &ResponseDecoder{maxLineBytes: maxLineBytes, maxValueBytes: maxValueBytes}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

// Decode 解码一条完整 ASCII response。
func (d *ResponseDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	line, lineBytes, ok, err := findLine(in, d.maxLineBytes)
	if err != nil || !ok {
		return nil, err
	}
	if line == string(StatusEnd) {
		if err := in.SkipBytes(lineBytes); err != nil {
			return nil, err
		}
		resp := Response{Status: StatusEnd, Values: d.values}
		d.values = nil
		return resp, nil
	}
	if strings.HasPrefix(line, "VALUE ") {
		return d.decodeValue(in, line, lineBytes)
	}
	if err := in.SkipBytes(lineBytes); err != nil {
		return nil, err
	}
	return parseResponseLine(line), nil
}

// ChannelInactive 释放尚未遇到 END 的 VALUE 数据。
func (d *ResponseDecoder) ChannelInactive(ctx *channel.HandlerContext) {
	for i := range d.values {
		d.values[i].Release()
	}
	d.values = nil
	d.ByteToMessageDecoder.ChannelInactive(ctx)
}

func (d *ResponseDecoder) decodeValue(in *buffer.CompositeByteBuf, line string, lineBytes int) (any, error) {
	value, err := parseValueLine(line)
	if err != nil {
		return nil, err
	}
	if value.Bytes < 0 || value.Bytes > d.maxValueBytes {
		return nil, codec.ErrFrameTooLong
	}
	total := lineBytes + value.Bytes + len(asciiCRLF)
	if in.ReadableBytes() < total {
		return nil, nil
	}
	reader := in.ReaderIndex()
	bodyEnd := reader + lineBytes + value.Bytes
	if !hasCRLFAt(in, bodyEnd) {
		return nil, codec.ErrInvalidFrameLength
	}
	if value.Bytes > 0 {
		value.Data, err = in.Slice(reader+lineBytes, value.Bytes)
		if err != nil {
			return nil, err
		}
	}
	if err := in.SkipBytes(total); err != nil {
		value.Release()
		return nil, err
	}
	d.values = append(d.values, Value{Key: value.Key, Flags: value.Flags, CAS: value.CAS, Data: value.Data})
	return nil, nil
}

type parsedValueLine struct {
	Key   string
	Flags uint32
	Bytes int
	CAS   uint64
	Data  buffer.ByteBuf
}

func (v parsedValueLine) Release() {
	if v.Data != nil {
		v.Data.Release()
	}
}

func parseValueLine(line string) (parsedValueLine, error) {
	parts := strings.Fields(line)
	if len(parts) != 4 && len(parts) != 5 {
		return parsedValueLine{}, codec.ErrInvalidFrameLength
	}
	flags, err := parseUint32Token(parts[2])
	if err != nil {
		return parsedValueLine{}, codec.ErrInvalidFrameLength
	}
	bytes, err := parseIntToken(parts[3])
	if err != nil || bytes < 0 {
		return parsedValueLine{}, codec.ErrInvalidFrameLength
	}
	var cas uint64
	if len(parts) == 5 {
		cas, err = parseUint64Token(parts[4])
		if err != nil {
			return parsedValueLine{}, codec.ErrInvalidFrameLength
		}
	}
	return parsedValueLine{Key: parts[1], Flags: flags, Bytes: bytes, CAS: cas}, nil
}

func parseResponseLine(line string) Response {
	status, msg, _ := strings.Cut(line, " ")
	return Response{Status: Status(status), Message: msg}
}

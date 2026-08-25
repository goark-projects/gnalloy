package memcache

import (
	"encoding/binary"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

type FrameDecoder struct {
	*codec.ByteToMessageDecoder
	maxFrameLength int
}

func NewFrameDecoder(maxFrameLength int) (*FrameDecoder, error) {
	if maxFrameLength < HeaderLength {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &FrameDecoder{maxFrameLength: maxFrameLength}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

func (d *FrameDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if in.ReadableBytes() < HeaderLength {
		return nil, nil
	}
	reader := in.ReaderIndex()
	magic, ok := in.GetByte(reader)
	if !ok || (magic != MagicRequest && magic != MagicResponse) {
		return nil, ErrInvalidFrame
	}
	opcodeByte, _ := in.GetByte(reader + 1)
	keyLength, err := in.ReadUnsigned(reader+2, 2, buffer.BigEndian)
	if err != nil {
		return nil, err
	}
	extrasLengthByte, _ := in.GetByte(reader + 4)
	dataType, _ := in.GetByte(reader + 5)
	vbucketOrStatus, err := in.ReadUnsigned(reader+6, 2, buffer.BigEndian)
	if err != nil {
		return nil, err
	}
	bodyLength, err := in.ReadUnsigned(reader+8, 4, buffer.BigEndian)
	if err != nil {
		return nil, err
	}
	opaque, err := in.ReadUnsigned(reader+12, 4, buffer.BigEndian)
	if err != nil {
		return nil, err
	}
	cas, err := in.ReadUnsigned(reader+16, 8, buffer.BigEndian)
	if err != nil {
		return nil, err
	}
	totalLength := HeaderLength + int(bodyLength)
	if totalLength > d.maxFrameLength {
		return nil, codec.ErrFrameTooLong
	}
	if in.ReadableBytes() < totalLength {
		return nil, nil
	}
	keyLen := int(keyLength)
	extrasLen := int(extrasLengthByte)
	if extrasLen+keyLen > int(bodyLength) {
		return nil, ErrInvalidFrame
	}
	frame := Frame{
		Magic:    magic,
		Opcode:   Opcode(opcodeByte),
		DataType: dataType,
		Opaque:   uint32(opaque),
		CAS:      cas,
	}
	if magic == MagicResponse {
		frame.Status = Status(vbucketOrStatus)
	} else {
		frame.VBucket = uint16(vbucketOrStatus)
	}
	bodyStart := reader + HeaderLength
	frame.Extras, err = slicePart(in, bodyStart, extrasLen)
	if err != nil {
		return nil, err
	}
	frame.Key, err = slicePart(in, bodyStart+extrasLen, keyLen)
	if err != nil {
		frame.Release()
		return nil, err
	}
	valueLen := int(bodyLength) - extrasLen - keyLen
	frame.Value, err = slicePart(in, bodyStart+extrasLen+keyLen, valueLen)
	if err != nil {
		frame.Release()
		return nil, err
	}
	if err := in.SkipBytes(totalLength); err != nil {
		frame.Release()
		return nil, err
	}
	return frame, nil
}

type FrameEncoder struct{}

func NewFrameEncoder() *FrameEncoder {
	return &FrameEncoder{}
}

func (e *FrameEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	frame, ok := msg.(Frame)
	if !ok {
		if ptr, ptrOK := msg.(*Frame); ptrOK && ptr != nil {
			frame = *ptr
			ok = true
		}
	}
	if !ok {
		return ctx.Write(msg)
	}
	if !frame.Valid() || readable(frame.Extras) > 0xff || readable(frame.Key) > 0xffff {
		frame.Release()
		return ErrInvalidFrame
	}
	header, err := ctx.Channel().Allocator().Acquire(HeaderLength)
	if err != nil {
		frame.Release()
		return err
	}
	var tmp [HeaderLength]byte
	tmp[0] = frame.Magic
	tmp[1] = byte(frame.Opcode)
	binary.BigEndian.PutUint16(tmp[2:4], uint16(readable(frame.Key)))
	tmp[4] = byte(readable(frame.Extras))
	tmp[5] = frame.DataType
	if frame.Magic == MagicResponse {
		binary.BigEndian.PutUint16(tmp[6:8], uint16(frame.Status))
	} else {
		binary.BigEndian.PutUint16(tmp[6:8], frame.VBucket)
	}
	binary.BigEndian.PutUint32(tmp[8:12], uint32(frame.BodyLength()))
	binary.BigEndian.PutUint32(tmp[12:16], frame.Opaque)
	binary.BigEndian.PutUint64(tmp[16:24], frame.CAS)
	if _, err := header.WriteBytes(tmp[:]); err != nil {
		header.Release()
		frame.Release()
		return err
	}
	if err := ctx.Write(header); err != nil {
		header.Release()
		frame.Release()
		return err
	}
	if err := writePart(ctx, frame.Extras); err != nil {
		frame.Release()
		return err
	}
	frame.Extras = nil
	if err := writePart(ctx, frame.Key); err != nil {
		frame.Release()
		return err
	}
	frame.Key = nil
	if err := writePart(ctx, frame.Value); err != nil {
		frame.Release()
		return err
	}
	frame.Value = nil
	return nil
}

func slicePart(in *buffer.CompositeByteBuf, index int, length int) (buffer.ByteBuf, error) {
	if length == 0 {
		return nil, nil
	}
	return in.Slice(index, length)
}

func writePart(ctx *channel.HandlerContext, buf buffer.ByteBuf) error {
	if buf == nil {
		return nil
	}
	if err := ctx.Write(buf); err != nil {
		buf.Release()
		return err
	}
	return nil
}

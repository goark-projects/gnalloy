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
	header, err := decodeBinaryHeader(in, reader)
	if err != nil {
		return nil, err
	}
	if header.magic != MagicRequest && header.magic != MagicResponse {
		return nil, ErrInvalidFrame
	}
	totalLength := HeaderLength + int(header.bodyLength)
	if totalLength > d.maxFrameLength {
		return nil, codec.ErrFrameTooLong
	}
	if in.ReadableBytes() < totalLength {
		return nil, nil
	}
	keyLen := int(header.keyLength)
	extrasLen := int(header.extrasLength)
	if extrasLen+keyLen > int(header.bodyLength) {
		return nil, ErrInvalidFrame
	}
	frame := Frame{
		Magic:    header.magic,
		Opcode:   Opcode(header.opcode),
		DataType: header.dataType,
		Opaque:   header.opaque,
		CAS:      header.cas,
	}
	if header.magic == MagicResponse {
		frame.Status = Status(header.vbucketOrStatus)
	} else {
		frame.VBucket = header.vbucketOrStatus
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
	valueLen := int(header.bodyLength) - extrasLen - keyLen
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

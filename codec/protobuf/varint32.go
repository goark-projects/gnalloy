package protobuf

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

const maxVarint32Bytes = 5

type Varint32FrameDecoder struct {
	*codec.ByteToMessageDecoder
	maxFrameLength int
}

type ProtobufVarint32FrameDecoder = Varint32FrameDecoder

func NewVarint32FrameDecoder(maxFrameLength int) (*Varint32FrameDecoder, error) {
	if maxFrameLength <= 0 {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &Varint32FrameDecoder{maxFrameLength: maxFrameLength}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

func NewProtobufVarint32FrameDecoder(maxFrameLength int) (*ProtobufVarint32FrameDecoder, error) {
	return NewVarint32FrameDecoder(maxFrameLength)
}

func (d *Varint32FrameDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	reader := in.ReaderIndex()
	length, header, ok, err := readRawVarint32(in)
	if err != nil || !ok {
		return nil, err
	}
	if length > d.maxFrameLength {
		return nil, codec.NewTooLongFrameError(length, d.maxFrameLength, 0)
	}
	total := header + length
	if in.ReadableBytes() < total {
		_ = in.SetReaderIndex(reader)
		return nil, nil
	}
	if err := in.SkipBytes(header); err != nil {
		return nil, err
	}
	frame, err := in.Slice(in.ReaderIndex(), length)
	if err != nil {
		return nil, err
	}
	if err := in.SkipBytes(length); err != nil {
		frame.Release()
		return nil, err
	}
	return frame, nil
}

type Varint32LengthFieldPrepender struct{}

type ProtobufVarint32LengthFieldPrepender = Varint32LengthFieldPrepender

func NewVarint32LengthFieldPrepender() *Varint32LengthFieldPrepender {
	return &Varint32LengthFieldPrepender{}
}

func NewProtobufVarint32LengthFieldPrepender() *ProtobufVarint32LengthFieldPrepender {
	return NewVarint32LengthFieldPrepender()
}

func (p *Varint32LengthFieldPrepender) Write(ctx *channel.HandlerContext, msg any) error {
	in, ok := msg.(buffer.ByteBuf)
	if !ok {
		return ctx.Write(msg)
	}
	length := in.ReadableBytes()
	if length < 0 {
		in.Release()
		return codec.ErrInvalidFrameLength
	}
	headerSize := rawVarint32Size(uint32(length))
	header, err := ctx.Channel().Allocator().Acquire(headerSize)
	if err != nil {
		in.Release()
		return err
	}
	var tmp [maxVarint32Bytes]byte
	n := putRawVarint32(tmp[:], uint32(length))
	if _, err := header.WriteBytes(tmp[:n]); err != nil {
		header.Release()
		in.Release()
		return err
	}
	if err := ctx.Write(header); err != nil {
		header.Release()
		in.Release()
		return err
	}
	return ctx.Write(in)
}

func readRawVarint32(in *buffer.CompositeByteBuf) (int, int, bool, error) {
	var result uint32
	for i := 0; i < maxVarint32Bytes; i++ {
		if in.ReadableBytes() <= i {
			return 0, 0, false, nil
		}
		b, ok := in.GetByte(in.ReaderIndex() + i)
		if !ok {
			return 0, 0, false, nil
		}
		result |= uint32(b&0x7f) << (7 * i)
		if b&0x80 == 0 {
			return int(result), i + 1, true, nil
		}
	}
	return 0, 0, false, codec.ErrInvalidLengthField
}

func rawVarint32Size(v uint32) int {
	size := 1
	for v >= 0x80 {
		v >>= 7
		size++
	}
	return size
}

func putRawVarint32(dst []byte, v uint32) int {
	i := 0
	for v >= 0x80 {
		dst[i] = byte(v) | 0x80
		v >>= 7
		i++
	}
	dst[i] = byte(v)
	return i + 1
}

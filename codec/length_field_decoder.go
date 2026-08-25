package codec

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

type LengthFieldBasedFrameDecoder struct {
	*ByteToMessageDecoder

	maxFrameLength     int
	lengthFieldOffset  int
	lengthFieldLength  int
	lengthAdjustment   int
	initialBytesToSkip int
	byteOrder          buffer.ByteOrder
}

func NewLengthFieldBasedFrameDecoder(maxFrameLength int, lengthFieldOffset int, lengthFieldLength int, lengthAdjustment int, initialBytesToSkip int, order buffer.ByteOrder) (*LengthFieldBasedFrameDecoder, error) {
	if maxFrameLength <= 0 || lengthFieldOffset < 0 || initialBytesToSkip < 0 {
		return nil, ErrInvalidLengthField
	}
	switch lengthFieldLength {
	case 1, 2, 3, 4, 8:
	default:
		return nil, ErrInvalidLengthField
	}
	d := &LengthFieldBasedFrameDecoder{
		maxFrameLength:     maxFrameLength,
		lengthFieldOffset:  lengthFieldOffset,
		lengthFieldLength:  lengthFieldLength,
		lengthAdjustment:   lengthAdjustment,
		initialBytesToSkip: initialBytesToSkip,
		byteOrder:          order,
	}
	d.ByteToMessageDecoder = NewByteToMessageDecoder(d)
	return d, nil
}

func (d *LengthFieldBasedFrameDecoder) Decode(_ *channel.HandlerContext, cumulation *buffer.CompositeByteBuf) (any, error) {
	if cumulation == nil {
		return nil, nil
	}
	readable := cumulation.ReadableBytes()
	minFrameLength := d.lengthFieldOffset + d.lengthFieldLength
	if readable < minFrameLength {
		return nil, nil
	}

	readerIndex := cumulation.ReaderIndex()
	lengthFieldIndex := readerIndex + d.lengthFieldOffset
	rawLength, err := cumulation.ReadUnsigned(lengthFieldIndex, d.lengthFieldLength, d.byteOrder)
	if err != nil {
		return nil, err
	}
	frameLength64 := int64(rawLength) + int64(d.lengthAdjustment) + int64(minFrameLength)
	if frameLength64 < int64(minFrameLength) || frameLength64 < int64(d.initialBytesToSkip) {
		return nil, ErrInvalidLengthField
	}
	if frameLength64 > int64(d.maxFrameLength) {
		return nil, ErrFrameTooLong
	}
	frameLength := int(frameLength64)
	if readable < frameLength {
		return nil, nil
	}

	frameIndex := readerIndex + d.initialBytesToSkip
	frameBytes := frameLength - d.initialBytesToSkip
	frame, err := cumulation.Slice(frameIndex, frameBytes)
	if err != nil {
		return nil, err
	}
	if err := cumulation.SkipBytes(frameLength); err != nil {
		frame.Release()
		return nil, err
	}
	return frame, nil
}

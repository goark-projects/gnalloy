package codec

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

type LengthFieldBasedFrameDecoder struct {
	maxFrameLength     int
	lengthFieldOffset  int
	lengthFieldLength  int
	lengthAdjustment   int
	initialBytesToSkip int
	byteOrder          buffer.ByteOrder

	cumulation *buffer.CompositeByteBuf
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
	return &LengthFieldBasedFrameDecoder{
		maxFrameLength:     maxFrameLength,
		lengthFieldOffset:  lengthFieldOffset,
		lengthFieldLength:  lengthFieldLength,
		lengthAdjustment:   lengthAdjustment,
		initialBytesToSkip: initialBytesToSkip,
		byteOrder:          order,
	}, nil
}

func (d *LengthFieldBasedFrameDecoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	in, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if d.cumulation == nil {
		d.cumulation = buffer.NewCompositeByteBuf()
	}
	d.cumulation.Append(in)

	for {
		frame, err := d.decode()
		if err != nil {
			ctx.FireExceptionCaught(err)
			d.releaseCumulation()
			return
		}
		if frame == nil {
			return
		}
		ctx.FireChannelRead(frame)
	}
}

func (d *LengthFieldBasedFrameDecoder) ChannelInactive(ctx *channel.HandlerContext) {
	d.releaseCumulation()
	ctx.FireChannelInactive()
}

func (d *LengthFieldBasedFrameDecoder) decode() (buffer.ByteBuf, error) {
	cumulation := d.cumulation
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
	cumulation.DiscardReadComponents()
	return frame, nil
}

func (d *LengthFieldBasedFrameDecoder) releaseCumulation() {
	if d.cumulation == nil {
		return
	}
	d.cumulation.Release()
	d.cumulation = nil
}

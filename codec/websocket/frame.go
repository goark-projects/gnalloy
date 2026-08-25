package websocket

import (
	"encoding/binary"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

const (
	OpcodeContinuation = 0x0
	OpcodeText         = 0x1
	OpcodeBinary       = 0x2
	OpcodeClose        = 0x8
	OpcodePing         = 0x9
	OpcodePong         = 0xa
)

type Frame struct {
	Final   bool
	Opcode  byte
	Payload buffer.ByteBuf
	Masked  bool
	MaskKey [4]byte
}

func (f Frame) Release() {
	if f.Payload != nil {
		f.Payload.Release()
	}
}

type FrameDecoder struct {
	*codec.ByteToMessageDecoder
	maxFrameLength     int
	expectMaskedFrames bool
	allowMaskedFrames  bool
}

func NewFrameDecoder(maxFrameLength int) (*FrameDecoder, error) {
	return newFrameDecoder(maxFrameLength, false, true)
}

func NewServerFrameDecoder(maxFrameLength int) (*FrameDecoder, error) {
	return newFrameDecoder(maxFrameLength, true, true)
}

func NewClientFrameDecoder(maxFrameLength int) (*FrameDecoder, error) {
	return newFrameDecoder(maxFrameLength, false, false)
}

func NewFrameDecoderWithMaskPolicy(maxFrameLength int, expectMaskedFrames bool, allowMaskedFrames bool) (*FrameDecoder, error) {
	return newFrameDecoder(maxFrameLength, expectMaskedFrames, allowMaskedFrames)
}

func newFrameDecoder(maxFrameLength int, expectMaskedFrames bool, allowMaskedFrames bool) (*FrameDecoder, error) {
	if maxFrameLength <= 0 {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &FrameDecoder{
		maxFrameLength:     maxFrameLength,
		expectMaskedFrames: expectMaskedFrames,
		allowMaskedFrames:  allowMaskedFrames,
	}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

func (d *FrameDecoder) Decode(ctx *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if in.ReadableBytes() < 2 {
		return nil, nil
	}
	reader := in.ReaderIndex()
	b0, _ := in.GetByte(reader)
	b1, _ := in.GetByte(reader + 1)
	opcode := b0 & 0x0f
	final := b0&0x80 != 0
	if b0&0x70 != 0 || !isKnownOpcode(opcode) {
		return nil, codec.ErrInvalidFrameLength
	}
	masked := b1&0x80 != 0
	if d.expectMaskedFrames && !masked {
		return nil, ErrMaskMismatch
	}
	if !d.allowMaskedFrames && masked {
		return nil, ErrMaskMismatch
	}
	payloadLength := int(b1 & 0x7f)
	headerLength := 2
	if payloadLength == 126 {
		if in.ReadableBytes() < 4 {
			return nil, nil
		}
		n, err := in.ReadUnsigned(reader+2, 2, buffer.BigEndian)
		if err != nil {
			return nil, err
		}
		payloadLength = int(n)
		headerLength = 4
	} else if payloadLength == 127 {
		if in.ReadableBytes() < 10 {
			return nil, nil
		}
		n, err := in.ReadUnsigned(reader+2, 8, buffer.BigEndian)
		if err != nil {
			return nil, err
		}
		if n > uint64(^uint(0)>>1) {
			return nil, codec.ErrFrameTooLong
		}
		payloadLength = int(n)
		headerLength = 10
	}
	if payloadLength > d.maxFrameLength {
		return nil, codec.ErrFrameTooLong
	}
	if isControlOpcode(opcode) {
		if !final || payloadLength > 125 || (opcode == OpcodeClose && payloadLength == 1) {
			return nil, ErrControlFrameInvalid
		}
	}
	if masked {
		headerLength += 4
	}
	total := headerLength + payloadLength
	if in.ReadableBytes() < total {
		return nil, nil
	}
	var mask [4]byte
	if masked {
		for i := 0; i < 4; i++ {
			mask[i], _ = in.GetByte(reader + headerLength - 4 + i)
		}
	}
	var payload buffer.ByteBuf
	if payloadLength > 0 {
		part, err := in.Slice(reader+headerLength, payloadLength)
		if err != nil {
			return nil, err
		}
		if masked {
			payload, err = unmask(part, mask)
			if err != nil {
				return nil, err
			}
		} else {
			payload = part
		}
	}
	if opcode == OpcodeClose && payload != nil && payload.ReadableBytes() >= 2 {
		status := binary.BigEndian.Uint16(payload.Bytes()[:2])
		if !IsValidCloseStatusCode(status) {
			payload.Release()
			return nil, ErrCloseStatusInvalid
		}
	}
	if err := in.SkipBytes(total); err != nil {
		if payload != nil {
			payload.Release()
		}
		return nil, err
	}
	return Frame{Final: final, Opcode: opcode, Payload: payload, Masked: masked, MaskKey: mask}, nil
}

type FrameEncoder struct{}

func NewFrameEncoder() *FrameEncoder {
	return &FrameEncoder{}
}

func (e *FrameEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	frame, ok := msg.(Frame)
	if !ok {
		return ctx.Write(msg)
	}
	payloadLength := 0
	if frame.Payload != nil {
		payloadLength = frame.Payload.ReadableBytes()
	}
	if err := validateOutboundFrame(frame, payloadLength); err != nil {
		if frame.Payload != nil {
			frame.Payload.Release()
		}
		return err
	}
	headerLength := websocketHeaderLength(payloadLength, frame.Masked)
	header, err := ctx.Channel().Allocator().Acquire(headerLength)
	if err != nil {
		if frame.Payload != nil {
			frame.Payload.Release()
		}
		return err
	}
	if err := writeWebSocketHeader(header, frame, payloadLength); err != nil {
		header.Release()
		if frame.Payload != nil {
			frame.Payload.Release()
		}
		return err
	}
	if err := ctx.Write(header); err != nil {
		header.Release()
		if frame.Payload != nil {
			frame.Payload.Release()
		}
		return err
	}
	if frame.Payload == nil {
		return nil
	}
	if !frame.Masked {
		return codec.WriteOutboundBuffer(ctx, frame.Payload)
	}
	masked, err := maskCopy(ctx, frame.Payload, frame.MaskKey)
	frame.Payload.Release()
	if err != nil {
		return err
	}
	return codec.WriteOutboundBuffer(ctx, masked)
}

func unmask(in buffer.ByteBuf, key [4]byte) (buffer.ByteBuf, error) {
	offset := 0
	for _, data := range in.ReadableSlices(nil) {
		for i := range data {
			data[i] ^= key[(offset+i)&3]
		}
		offset += len(data)
	}
	return in, nil
}

func maskCopy(ctx *channel.HandlerContext, in buffer.ByteBuf, key [4]byte) (buffer.ByteBuf, error) {
	out, err := ctx.Channel().Allocator().Acquire(in.ReadableBytes())
	if err != nil {
		return nil, err
	}
	if _, err := out.WriteBytes(in.Bytes()); err != nil {
		out.Release()
		return nil, err
	}
	data := out.Bytes()
	for i := range data {
		data[i] ^= key[i&3]
	}
	return out, nil
}

func websocketHeaderLength(payloadLength int, masked bool) int {
	n := 2
	if payloadLength >= 126 && payloadLength <= 0xffff {
		n += 2
	} else if payloadLength > 0xffff {
		n += 8
	}
	if masked {
		n += 4
	}
	return n
}

func writeWebSocketHeader(out buffer.ByteBuf, frame Frame, payloadLength int) error {
	b0 := frame.Opcode & 0x0f
	if frame.Final {
		b0 |= 0x80
	}
	if _, err := out.WriteBytes([]byte{b0}); err != nil {
		return err
	}
	maskBit := byte(0)
	if frame.Masked {
		maskBit = 0x80
	}
	if payloadLength < 126 {
		if _, err := out.WriteBytes([]byte{maskBit | byte(payloadLength)}); err != nil {
			return err
		}
	} else if payloadLength <= 0xffff {
		if _, err := out.WriteBytes([]byte{maskBit | 126, byte(payloadLength >> 8), byte(payloadLength)}); err != nil {
			return err
		}
	} else {
		var tmp [9]byte
		tmp[0] = maskBit | 127
		n := uint64(payloadLength)
		for i := 0; i < 8; i++ {
			tmp[1+i] = byte(n >> (8 * (7 - i)))
		}
		if _, err := out.WriteBytes(tmp[:]); err != nil {
			return err
		}
	}
	if frame.Masked {
		_, err := out.WriteBytes(frame.MaskKey[:])
		return err
	}
	return nil
}

func validateOutboundFrame(frame Frame, payloadLength int) error {
	if frame.Opcode == OpcodeClose && payloadLength >= 2 && frame.Payload != nil {
		status := binary.BigEndian.Uint16(frame.Payload.Bytes()[:2])
		if !IsValidCloseStatusCode(status) {
			return ErrCloseStatusInvalid
		}
	}
	if isControlOpcode(frame.Opcode) && (!frame.Final || payloadLength > 125 || (frame.Opcode == OpcodeClose && payloadLength == 1)) {
		return ErrControlFrameInvalid
	}
	return nil
}

func IsValidCloseStatusCode(code uint16) bool {
	if code < 1000 || code >= 5000 {
		return false
	}
	switch code {
	case 1004, 1005, 1006, 1015:
		return false
	}
	if code >= 1016 && code < 3000 {
		return false
	}
	return true
}

func isKnownOpcode(opcode byte) bool {
	switch opcode {
	case OpcodeContinuation, OpcodeText, OpcodeBinary, OpcodeClose, OpcodePing, OpcodePong:
		return true
	default:
		return false
	}
}

func isControlOpcode(opcode byte) bool {
	return opcode >= 0x8
}

func isDataOpcode(opcode byte) bool {
	return opcode == OpcodeText || opcode == OpcodeBinary
}

package redis

import (
	"strconv"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

type FrameDecoder struct {
	*codec.ByteToMessageDecoder
	maxFrameLength int
}

func NewFrameDecoder(maxFrameLength int) (*FrameDecoder, error) {
	if maxFrameLength <= 0 {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &FrameDecoder{maxFrameLength: maxFrameLength}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

func (d *FrameDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	length, ok, err := respFrameLength(in, in.ReaderIndex())
	if err != nil || !ok {
		return nil, err
	}
	if length > d.maxFrameLength {
		return nil, codec.ErrFrameTooLong
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

type CommandEncoder struct{}

func NewCommandEncoder() *CommandEncoder {
	return &CommandEncoder{}
}

func Command(args ...string) [][]byte {
	out := make([][]byte, len(args))
	for i, arg := range args {
		out[i] = []byte(arg)
	}
	return out
}

func CommandBytes(args ...[]byte) [][]byte {
	return args
}

func (e *CommandEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	args, ok := msg.([][]byte)
	if !ok {
		return ctx.Write(msg)
	}
	size := 1 + len(strconv.Itoa(len(args))) + 2
	for _, arg := range args {
		size += 1 + len(strconv.Itoa(len(arg))) + 2 + len(arg) + 2
	}
	out, err := ctx.Channel().Allocator().Acquire(size)
	if err != nil {
		return err
	}
	if _, err := out.WriteBytes([]byte("*" + strconv.Itoa(len(args)) + "\r\n")); err != nil {
		out.Release()
		return err
	}
	for _, arg := range args {
		if _, err := out.WriteBytes([]byte("$" + strconv.Itoa(len(arg)) + "\r\n")); err != nil {
			out.Release()
			return err
		}
		if _, err := out.WriteBytes(arg); err != nil {
			out.Release()
			return err
		}
		if _, err := out.WriteBytes([]byte("\r\n")); err != nil {
			out.Release()
			return err
		}
	}
	return codec.WriteOutboundBuffer(ctx, out)
}

func respFrameLength(in *buffer.CompositeByteBuf, index int) (int, bool, error) {
	if in.WriterIndex()-index < 3 {
		return 0, false, nil
	}
	prefix, ok := in.GetByte(index)
	if !ok {
		return 0, false, nil
	}
	lineEnd, ok := findCRLF(in, index+1)
	if !ok {
		return 0, false, nil
	}
	lineLength := lineEnd - index + 2
	switch prefix {
	case '+', '-', ':':
		return lineLength, true, nil
	case '$':
		n, err := parseRESPInt(in, index+1, lineEnd)
		if err != nil {
			return 0, false, err
		}
		if n < 0 {
			return lineLength, true, nil
		}
		total := lineLength + n + 2
		if in.WriterIndex()-index < total {
			return 0, false, nil
		}
		return total, true, nil
	case '*':
		count, err := parseRESPInt(in, index+1, lineEnd)
		if err != nil {
			return 0, false, err
		}
		if count < 0 {
			return lineLength, true, nil
		}
		total := lineLength
		for i := 0; i < count; i++ {
			n, ok, err := respFrameLength(in, index+total)
			if err != nil || !ok {
				return 0, ok, err
			}
			total += n
		}
		return total, true, nil
	default:
		return 0, false, codec.ErrInvalidFrameLength
	}
}

func findCRLF(in *buffer.CompositeByteBuf, start int) (int, bool) {
	for i := start; i+1 < in.WriterIndex(); i++ {
		a, ok := in.GetByte(i)
		if !ok || a != '\r' {
			continue
		}
		b, ok := in.GetByte(i + 1)
		if ok && b == '\n' {
			return i, true
		}
	}
	return 0, false
}

func parseRESPInt(in *buffer.CompositeByteBuf, start int, end int) (int, error) {
	if start >= end {
		return 0, codec.ErrInvalidFrameLength
	}
	sign := 1
	if b, _ := in.GetByte(start); b == '-' {
		sign = -1
		start++
	}
	n := 0
	for i := start; i < end; i++ {
		b, ok := in.GetByte(i)
		if !ok || b < '0' || b > '9' {
			return 0, codec.ErrInvalidFrameLength
		}
		n = n*10 + int(b-'0')
	}
	return n * sign, nil
}

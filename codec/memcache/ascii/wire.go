package ascii

import (
	"strconv"
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

var asciiCRLF = []byte{'\r', '\n'}

func findLine(in *buffer.CompositeByteBuf, maxLineBytes int) (string, int, bool, error) {
	reader := in.ReaderIndex()
	lineEnd, ok := in.Index(reader, asciiCRLF)
	if !ok {
		if in.ReadableBytes() > maxLineBytes {
			return "", 0, false, codec.ErrFrameTooLong
		}
		return "", 0, false, nil
	}
	lineBytes := lineEnd - reader
	if lineBytes > maxLineBytes {
		return "", 0, false, codec.ErrFrameTooLong
	}
	line, err := buffer.ReadableStringAt(in, reader, lineBytes)
	if err != nil {
		return "", 0, false, err
	}
	return line, lineBytes + len(asciiCRLF), true, nil
}

func parseUint32Token(value string) (uint32, error) {
	parsed, err := strconv.ParseUint(value, 10, 32)
	return uint32(parsed), err
}

func parseUint64Token(value string) (uint64, error) {
	return strconv.ParseUint(value, 10, 64)
}

func parseInt64Token(value string) (int64, error) {
	return strconv.ParseInt(value, 10, 64)
}

func parseIntToken(value string) (int, error) {
	parsed, err := strconv.ParseInt(value, 10, 32)
	return int(parsed), err
}

func writeASCIIBytes(ctx *channel.HandlerContext, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	out, err := ctx.Channel().Allocator().Acquire(len(data))
	if err != nil {
		return err
	}
	if _, err := out.WriteBytes(data); err != nil {
		out.Release()
		return err
	}
	return codec.WriteOutboundBuffer(ctx, out)
}

func writeASCIIString(ctx *channel.HandlerContext, data string) error {
	return writeASCIIBytes(ctx, []byte(data))
}

func commandLower(value string) Command {
	return Command(strings.ToLower(value))
}

func readable(buf buffer.ByteBuf) int {
	if buf == nil {
		return 0
	}
	return buf.ReadableBytes()
}

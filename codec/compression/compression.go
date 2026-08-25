package compression

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

const DefaultMaxDecodedBytes = 32 << 20

type Format uint8

const (
	FormatGzip Format = iota + 1
	FormatZlib
)

// Encoder 把入站 ByteBuf 压缩后继续写出。
type Encoder struct {
	format Format
	level  int
}

func NewGzipEncoder(level int) (*Encoder, error) {
	return newEncoder(FormatGzip, level)
}

func NewZlibEncoder(level int) (*Encoder, error) {
	return newEncoder(FormatZlib, level)
}

func (e *Encoder) Write(ctx *channel.HandlerContext, msg any) error {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return ctx.Write(msg)
	}
	out, err := encode(ctx.Channel().Allocator(), e.format, e.level, buf.Bytes())
	buf.Release()
	if err != nil {
		return err
	}
	if err := ctx.Write(out); err != nil {
		out.Release()
		return err
	}
	return nil
}

// Decoder 把压缩 ByteBuf 解压后继续入站传播。
type Decoder struct {
	format          Format
	maxDecodedBytes int
}

func NewGzipDecoder(maxDecodedBytes int) (*Decoder, error) {
	return newDecoder(FormatGzip, maxDecodedBytes)
}

func NewZlibDecoder(maxDecodedBytes int) (*Decoder, error) {
	return newDecoder(FormatZlib, maxDecodedBytes)
}

func (d *Decoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	out, err := decode(ctx.Channel().Allocator(), d.format, d.maxDecodedBytes, buf.Bytes())
	buf.Release()
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(out)
}

func newEncoder(format Format, level int) (*Encoder, error) {
	if !validFormat(format) || !validLevel(level) {
		return nil, ErrInvalidConfig
	}
	return &Encoder{format: format, level: level}, nil
}

func newDecoder(format Format, maxDecodedBytes int) (*Decoder, error) {
	if !validFormat(format) || maxDecodedBytes < 0 {
		return nil, ErrInvalidConfig
	}
	if maxDecodedBytes == 0 {
		maxDecodedBytes = DefaultMaxDecodedBytes
	}
	return &Decoder{format: format, maxDecodedBytes: maxDecodedBytes}, nil
}

func encode(alloc buffer.Allocator, format Format, level int, src []byte) (buffer.ByteBuf, error) {
	if alloc == nil {
		return nil, ErrInvalidConfig
	}
	var dst bytes.Buffer
	var writer io.WriteCloser
	var err error
	switch format {
	case FormatGzip:
		writer, err = gzip.NewWriterLevel(&dst, level)
	case FormatZlib:
		writer, err = zlib.NewWriterLevel(&dst, level)
	default:
		err = ErrInvalidConfig
	}
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(src); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return byteBufFromBytes(alloc, dst.Bytes())
}

func decode(alloc buffer.Allocator, format Format, maxDecodedBytes int, src []byte) (buffer.ByteBuf, error) {
	if alloc == nil {
		return nil, ErrInvalidConfig
	}
	reader, err := newReader(format, bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	limited := io.LimitReader(reader, int64(maxDecodedBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > maxDecodedBytes {
		return nil, ErrDecodedTooLong
	}
	return byteBufFromBytes(alloc, data)
}

func newReader(format Format, src io.Reader) (io.ReadCloser, error) {
	switch format {
	case FormatGzip:
		return gzip.NewReader(src)
	case FormatZlib:
		return zlib.NewReader(src)
	default:
		return nil, ErrInvalidConfig
	}
}

func byteBufFromBytes(alloc buffer.Allocator, data []byte) (buffer.ByteBuf, error) {
	size := len(data)
	if size == 0 {
		size = 1
	}
	out, err := alloc.Acquire(size)
	if err != nil {
		return nil, err
	}
	if len(data) > 0 {
		if _, err := out.WriteBytes(data); err != nil {
			out.Release()
			return nil, err
		}
	}
	return out, nil
}

func validFormat(format Format) bool {
	return format == FormatGzip || format == FormatZlib
}

func validLevel(level int) bool {
	return level == flate.DefaultCompression ||
		level == flate.HuffmanOnly ||
		(level >= flate.NoCompression && level <= flate.BestCompression)
}

package compression

import (
	"compress/flate"
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
	format  Format
	level   int
	writers *compressionWriterPool
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
	out, err := e.encodeByteBuf(ctx.Channel().Allocator(), buf)
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
	readers         *compressionReaderPool
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
	out, err := d.decodeByteBuf(ctx.Channel().Allocator(), buf)
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
	return &Encoder{format: format, level: level, writers: newCompressionWriterPool(format, level)}, nil
}

func newDecoder(format Format, maxDecodedBytes int) (*Decoder, error) {
	if !validFormat(format) || maxDecodedBytes < 0 {
		return nil, ErrInvalidConfig
	}
	if maxDecodedBytes == 0 {
		maxDecodedBytes = DefaultMaxDecodedBytes
	}
	return &Decoder{format: format, maxDecodedBytes: maxDecodedBytes, readers: newCompressionReaderPool(format)}, nil
}

func encode(alloc buffer.Allocator, format Format, level int, src []byte) (buffer.ByteBuf, error) {
	encoder, err := newEncoder(format, level)
	if err != nil {
		return nil, err
	}
	return encoder.encodeBytes(alloc, src)
}

func decode(alloc buffer.Allocator, format Format, maxDecodedBytes int, src []byte) (buffer.ByteBuf, error) {
	decoder, err := newDecoder(format, maxDecodedBytes)
	if err != nil {
		return nil, err
	}
	return decoder.decodeReader(alloc, bytesReader(src))
}

func (e *Encoder) encodeByteBuf(alloc buffer.Allocator, src buffer.ByteBuf) (buffer.ByteBuf, error) {
	return e.encodeReadable(alloc, func(writer io.Writer) error {
		return writeByteBufTo(writer, src)
	})
}

func (e *Encoder) encodeBytes(alloc buffer.Allocator, src []byte) (buffer.ByteBuf, error) {
	return e.encodeReadable(alloc, func(writer io.Writer) error {
		_, err := writer.Write(src)
		return err
	})
}

func (e *Encoder) encodeReadable(alloc buffer.Allocator, write func(io.Writer) error) (buffer.ByteBuf, error) {
	if alloc == nil {
		return nil, ErrInvalidConfig
	}
	if e.writers == nil {
		e.writers = newCompressionWriterPool(e.format, e.level)
	}
	state, err := e.writers.acquire()
	if err != nil {
		return nil, err
	}
	defer e.writers.release(state)
	if err := write(state.writer); err != nil {
		_ = state.writer.Close()
		return nil, err
	}
	if err := state.writer.Close(); err != nil {
		return nil, err
	}
	return byteBufFromBytes(alloc, state.dst.Bytes())
}

func (d *Decoder) decodeByteBuf(alloc buffer.Allocator, src buffer.ByteBuf) (buffer.ByteBuf, error) {
	return d.decodeReader(alloc, newByteBufReadSource(src))
}

func (d *Decoder) decodeReader(alloc buffer.Allocator, src io.Reader) (buffer.ByteBuf, error) {
	if alloc == nil {
		return nil, ErrInvalidConfig
	}
	if d.readers == nil {
		d.readers = newCompressionReaderPool(d.format)
	}
	reader, err := d.readers.acquire(src)
	if err != nil {
		return nil, err
	}
	defer d.readers.release(reader)
	limited := io.LimitReader(reader, int64(d.maxDecodedBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > d.maxDecodedBytes {
		return nil, ErrDecodedTooLong
	}
	return byteBufFromBytes(alloc, data)
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

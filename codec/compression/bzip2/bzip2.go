package bzip2

import (
	"bytes"
	"io"
	"sync"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	base "goark.dev/gnalloy/codec/compression"
	"goark.dev/gnalloy/codec/compression/internal/stream"

	nativebzip2 "github.com/dsnet/compress/bzip2"
)

const (
	BestSpeed       = nativebzip2.BestSpeed
	BestCompression = nativebzip2.BestCompression
	DefaultLevel    = nativebzip2.DefaultCompression
	maxRetained     = 1 << 20
)

// Encoder 把 ByteBuf 编码为 BZip2 stream。
type Encoder struct {
	level int
	pool  sync.Pool
}

// NewEncoder 创建 BZip2 编码器。
func NewEncoder(level int) (*Encoder, error) {
	if level < BestSpeed || level > BestCompression {
		return nil, base.ErrInvalidConfig
	}
	return &Encoder{level: level}, nil
}

// Write 压缩 ByteBuf 并把压缩结果继续写出。
func (e *Encoder) Write(ctx *channel.HandlerContext, msg any) error {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return ctx.Write(msg)
	}
	out, err := e.encode(ctx.Channel().Allocator(), buf)
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

func (e *Encoder) encode(alloc buffer.Allocator, src buffer.ByteBuf) (buffer.ByteBuf, error) {
	state, err := e.acquireWriter()
	if err != nil {
		return nil, err
	}
	defer e.releaseWriter(state)
	if err := stream.WriteByteBuf(state.writer, src); err != nil {
		_ = state.writer.Close()
		return nil, err
	}
	if err := state.writer.Close(); err != nil {
		return nil, err
	}
	return stream.ByteBufFromBytes(alloc, state.dst.Bytes())
}

func (e *Encoder) acquireWriter() (*writerState, error) {
	if value := e.pool.Get(); value != nil {
		state := value.(*writerState)
		state.dst.Reset()
		if err := state.writer.Reset(&state.dst); err != nil {
			return nil, err
		}
		return state, nil
	}
	state := &writerState{}
	writer, err := nativebzip2.NewWriter(&state.dst, &nativebzip2.WriterConfig{Level: e.level})
	if err != nil {
		return nil, err
	}
	state.writer = writer
	return state, nil
}

func (e *Encoder) releaseWriter(state *writerState) {
	if state == nil {
		return
	}
	if state.dst.Cap() > maxRetained {
		state.dst = bytes.Buffer{}
	} else {
		state.dst.Reset()
	}
	e.pool.Put(state)
}

type writerState struct {
	dst    bytes.Buffer
	writer *nativebzip2.Writer
}

// Decoder 把 BZip2 stream 解码为 ByteBuf。
type Decoder struct {
	maxDecodedBytes int
	pool            sync.Pool
}

// NewDecoder 创建 BZip2 解码器。
func NewDecoder(maxDecodedBytes int) *Decoder {
	return &Decoder{maxDecodedBytes: maxDecodedBytes}
}

// ChannelRead 解压 ByteBuf 并继续传播。
func (d *Decoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	out, err := d.decode(ctx.Channel().Allocator(), buf)
	buf.Release()
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(out)
}

func (d *Decoder) decode(alloc buffer.Allocator, src buffer.ByteBuf) (buffer.ByteBuf, error) {
	reader, err := d.acquireReader(stream.NewByteBufReader(src))
	if err != nil {
		return nil, err
	}
	defer d.releaseReader(reader)
	return stream.DecodeAll(alloc, reader, d.maxDecodedBytes)
}

func (d *Decoder) acquireReader(src io.Reader) (*nativebzip2.Reader, error) {
	if value := d.pool.Get(); value != nil {
		reader := value.(*nativebzip2.Reader)
		if err := reader.Reset(src); err != nil {
			return nil, err
		}
		return reader, nil
	}
	return nativebzip2.NewReader(src, nil)
}

func (d *Decoder) releaseReader(reader *nativebzip2.Reader) {
	if reader == nil {
		return
	}
	_ = reader.Close()
	d.pool.Put(reader)
}

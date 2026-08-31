package zstd

import (
	"bytes"
	"sync"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	base "goark.dev/gnalloy/codec/compression"
	"goark.dev/gnalloy/codec/compression/internal/stream"

	nativezstd "github.com/klauspost/compress/zstd"
)

const maxRetained = 1 << 20

type EncoderLevel = nativezstd.EncoderLevel

const (
	SpeedFastest = nativezstd.SpeedFastest
	SpeedDefault = nativezstd.SpeedDefault
	SpeedBetter  = nativezstd.SpeedBetterCompression
	SpeedBest    = nativezstd.SpeedBestCompression
	DefaultLevel = SpeedDefault

	defaultWindowSize = 8 << 20
)

// Encoder 把 ByteBuf 编码为 Zstandard stream。
type Encoder struct {
	level EncoderLevel
	pool  sync.Pool
}

// NewEncoder 创建 Zstandard 编码器。
func NewEncoder(level EncoderLevel) (*Encoder, error) {
	if level < SpeedFastest || level > SpeedBest {
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
		state.writer.Close()
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
		state.writer.Reset(&state.dst)
		return state, nil
	}
	state := &writerState{}
	writer, err := nativezstd.NewWriter(
		&state.dst,
		nativezstd.WithEncoderLevel(e.level),
		nativezstd.WithEncoderConcurrency(1),
	)
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
	writer *nativezstd.Encoder
}

// Decoder 把 Zstandard stream 解码为 ByteBuf。
type Decoder struct {
	maxDecodedBytes int
	pool            sync.Pool
}

// NewDecoder 创建 Zstandard 解码器。
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
	reader, err := d.acquireReader(stream.NewByteBufReader(buf))
	if err != nil {
		buf.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	out, err := stream.DecodeAll(ctx.Channel().Allocator(), reader, d.maxDecodedBytes)
	buf.Release()
	d.releaseReader(reader)
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(out)
}

func (d *Decoder) acquireReader(src interface{ Read([]byte) (int, error) }) (*nativezstd.Decoder, error) {
	if value := d.pool.Get(); value != nil {
		reader := value.(*nativezstd.Decoder)
		if err := reader.Reset(src); err != nil {
			return nil, err
		}
		return reader, nil
	}
	return nativezstd.NewReader(
		src,
		nativezstd.WithDecoderConcurrency(1),
		nativezstd.WithDecoderMaxWindow(defaultWindowSize),
	)
}

func (d *Decoder) releaseReader(reader *nativezstd.Decoder) {
	if reader == nil {
		return
	}
	_ = reader.Reset(nil)
	d.pool.Put(reader)
}

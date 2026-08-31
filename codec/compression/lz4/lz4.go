package lz4

import (
	"bytes"
	"sync"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	base "goark.dev/gnalloy/codec/compression"
	"goark.dev/gnalloy/codec/compression/internal/stream"

	nativelz4 "github.com/pierrec/lz4/v4"
)

type CompressionLevel = nativelz4.CompressionLevel

const (
	Fast         = nativelz4.Fast
	Level1       = nativelz4.Level1
	Level2       = nativelz4.Level2
	Level3       = nativelz4.Level3
	Level4       = nativelz4.Level4
	Level5       = nativelz4.Level5
	Level6       = nativelz4.Level6
	Level7       = nativelz4.Level7
	Level8       = nativelz4.Level8
	Level9       = nativelz4.Level9
	DefaultLevel = Fast
	maxRetained  = 1 << 20
)

// Encoder 把 ByteBuf 编码为 LZ4 stream。
type Encoder struct {
	level CompressionLevel
	pool  sync.Pool
}

// NewEncoder 创建 LZ4 编码器。
func NewEncoder(level CompressionLevel) (*Encoder, error) {
	if !validLevel(level) {
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
		state.writer.Reset(&state.dst)
		if err := state.writer.Apply(nativelz4.CompressionLevelOption(e.level)); err != nil {
			return nil, err
		}
		return state, nil
	}
	state := &writerState{}
	state.writer = nativelz4.NewWriter(&state.dst)
	if err := state.writer.Apply(nativelz4.CompressionLevelOption(e.level)); err != nil {
		return nil, err
	}
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
	writer *nativelz4.Writer
}

// Decoder 把 LZ4 stream 解码为 ByteBuf。
type Decoder struct {
	maxDecodedBytes int
	pool            sync.Pool
}

// NewDecoder 创建 LZ4 解码器。
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
	reader := d.acquireReader(stream.NewByteBufReader(buf))
	out, err := stream.DecodeAll(ctx.Channel().Allocator(), reader, d.maxDecodedBytes)
	buf.Release()
	d.releaseReader(reader)
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(out)
}

func (d *Decoder) acquireReader(src interface{ Read([]byte) (int, error) }) *nativelz4.Reader {
	if value := d.pool.Get(); value != nil {
		reader := value.(*nativelz4.Reader)
		reader.Reset(src)
		return reader
	}
	return nativelz4.NewReader(src)
}

func (d *Decoder) releaseReader(reader *nativelz4.Reader) {
	if reader != nil {
		d.pool.Put(reader)
	}
}

func validLevel(level CompressionLevel) bool {
	switch level {
	case Fast, Level1, Level2, Level3, Level4, Level5, Level6, Level7, Level8, Level9:
		return true
	default:
		return false
	}
}

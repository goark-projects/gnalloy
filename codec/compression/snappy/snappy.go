package snappy

import (
	"bytes"
	"sync"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/compression/internal/stream"

	nativesnappy "github.com/golang/snappy"
)

const maxRetained = 1 << 20

// Encoder 把 ByteBuf 编码为 Snappy stream。
type Encoder struct {
	pool sync.Pool
}

// NewEncoder 创建 Snappy 编码器。
func NewEncoder() *Encoder {
	return &Encoder{}
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
	state := e.acquireWriter()
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

func (e *Encoder) acquireWriter() *writerState {
	if value := e.pool.Get(); value != nil {
		state := value.(*writerState)
		state.dst.Reset()
		state.writer.Reset(&state.dst)
		return state
	}
	state := &writerState{}
	state.writer = nativesnappy.NewBufferedWriter(&state.dst)
	return state
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
	writer *nativesnappy.Writer
}

// Decoder 把 Snappy stream 解码为 ByteBuf。
type Decoder struct {
	maxDecodedBytes int
	pool            sync.Pool
}

// NewDecoder 创建 Snappy 解码器。
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

func (d *Decoder) acquireReader(src interface{ Read([]byte) (int, error) }) *nativesnappy.Reader {
	if value := d.pool.Get(); value != nil {
		reader := value.(*nativesnappy.Reader)
		reader.Reset(src)
		return reader
	}
	return nativesnappy.NewReader(src)
}

func (d *Decoder) releaseReader(reader *nativesnappy.Reader) {
	if reader != nil {
		d.pool.Put(reader)
	}
}

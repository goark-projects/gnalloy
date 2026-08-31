package lzma

import (
	"bytes"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	base "goark.dev/gnalloy/codec/compression"
	"goark.dev/gnalloy/codec/compression/internal/stream"

	nativelzma "github.com/ulikunitz/xz/lzma"
)

const (
	DefaultDictCap = 1 << 20
	DefaultBufSize = 4096
)

// Config 描述 LZMA 编解码边界。
type Config struct {
	DictCap int
	BufSize int
}

// Encoder 把 ByteBuf 编码为 classic LZMA stream。
type Encoder struct {
	cfg Config
}

// NewEncoder 创建 LZMA 编码器。
func NewEncoder(cfg Config) (*Encoder, error) {
	cfg = normalizeConfig(cfg)
	if cfg.DictCap < nativelzma.MinDictCap || cfg.BufSize <= 0 {
		return nil, base.ErrInvalidConfig
	}
	return &Encoder{cfg: cfg}, nil
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
	var dst bytes.Buffer
	writer, err := nativelzma.WriterConfig{
		DictCap:   e.cfg.DictCap,
		BufSize:   e.cfg.BufSize,
		EOSMarker: true,
	}.NewWriter(&dst)
	if err != nil {
		return nil, err
	}
	if err := stream.WriteByteBuf(writer, src); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return stream.ByteBufFromBytes(alloc, dst.Bytes())
}

// Decoder 把 classic LZMA stream 解码为 ByteBuf。
type Decoder struct {
	maxDecodedBytes int
	cfg             Config
}

// NewDecoder 创建 LZMA 解码器。
func NewDecoder(maxDecodedBytes int, cfg Config) (*Decoder, error) {
	if maxDecodedBytes < 0 {
		return nil, base.ErrInvalidConfig
	}
	cfg = normalizeConfig(cfg)
	if cfg.DictCap < nativelzma.MinDictCap || cfg.BufSize <= 0 {
		return nil, base.ErrInvalidConfig
	}
	return &Decoder{maxDecodedBytes: maxDecodedBytes, cfg: cfg}, nil
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
	reader, err := nativelzma.ReaderConfig{DictCap: d.cfg.DictCap}.NewReader(stream.NewByteBufReader(src))
	if err != nil {
		return nil, err
	}
	return stream.DecodeAll(alloc, reader, d.maxDecodedBytes)
}

func normalizeConfig(cfg Config) Config {
	if cfg.DictCap == 0 {
		cfg.DictCap = DefaultDictCap
	}
	if cfg.BufSize == 0 {
		cfg.BufSize = DefaultBufSize
	}
	return cfg
}

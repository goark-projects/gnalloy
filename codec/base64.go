package codec

import (
	"encoding/base64"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

type Base64Dialect struct {
	Encoding *base64.Encoding
}

var (
	Base64StandardDialect = Base64Dialect{Encoding: base64.StdEncoding}
	Base64URLDialect      = Base64Dialect{Encoding: base64.URLEncoding}
)

type Base64Encoder struct {
	encoding *base64.Encoding
}

func NewBase64Encoder() *Base64Encoder {
	return NewBase64EncoderWithDialect(Base64StandardDialect)
}

func NewBase64EncoderWithDialect(dialect Base64Dialect) *Base64Encoder {
	encoding := dialect.Encoding
	if encoding == nil {
		encoding = base64.StdEncoding
	}
	return &Base64Encoder{encoding: encoding}
}

func (e *Base64Encoder) Write(ctx *channel.HandlerContext, msg any) error {
	in, ok := msg.(buffer.ByteBuf)
	if !ok {
		return ctx.Write(msg)
	}
	defer in.Release()
	out, err := ctx.Channel().Allocator().Acquire(e.encoding.EncodedLen(in.ReadableBytes()))
	if err != nil {
		return err
	}
	e.encoding.Encode(out.WritableBytesView(), in.Bytes())
	if err := out.AdvanceWriter(e.encoding.EncodedLen(in.ReadableBytes())); err != nil {
		out.Release()
		return err
	}
	return ctx.Write(out)
}

type Base64Decoder struct {
	encoding *base64.Encoding
}

func NewBase64Decoder() *Base64Decoder {
	return NewBase64DecoderWithDialect(Base64StandardDialect)
}

func NewBase64DecoderWithDialect(dialect Base64Dialect) *Base64Decoder {
	encoding := dialect.Encoding
	if encoding == nil {
		encoding = base64.StdEncoding
	}
	return &Base64Decoder{encoding: encoding}
}

func (d *Base64Decoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	in, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	defer in.Release()
	out, err := ctx.Channel().Allocator().Acquire(d.encoding.DecodedLen(in.ReadableBytes()))
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	n, err := d.encoding.Decode(out.WritableBytesView(), in.Bytes())
	if err != nil {
		out.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if err := out.AdvanceWriter(n); err != nil {
		out.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(out)
}

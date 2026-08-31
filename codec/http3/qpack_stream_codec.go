package http3

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

// QPACKEncoderStreamDecoder 解码对端 encoder stream 指令。
type QPACKEncoderStreamDecoder struct {
	*codec.ByteToMessageDecoder
}

// NewQPACKEncoderStreamDecoder 创建 encoder stream 指令解码器。
func NewQPACKEncoderStreamDecoder() *QPACKEncoderStreamDecoder {
	d := &QPACKEncoderStreamDecoder{}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d
}

// Decode 解码一条完整 encoder stream 指令。
func (d *QPACKEncoderStreamDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	msg, n, ok, err := decodeQPACKEncoderInstruction(in, in.ReaderIndex())
	if err != nil || !ok {
		return nil, err
	}
	if err := in.SkipBytes(n); err != nil {
		return nil, err
	}
	return msg, nil
}

// QPACKEncoderStreamEncoder 编码本端 encoder stream 指令。
type QPACKEncoderStreamEncoder struct{}

// NewQPACKEncoderStreamEncoder 创建 encoder stream 指令编码器。
func NewQPACKEncoderStreamEncoder() *QPACKEncoderStreamEncoder {
	return &QPACKEncoderStreamEncoder{}
}

// Write 编码 encoder stream 指令，其它消息透传。
func (e *QPACKEncoderStreamEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	data, ok, err := appendQPACKEncoderInstruction(nil, msg)
	if err != nil {
		return err
	}
	if !ok {
		return ctx.Write(msg)
	}
	return writeBytes(ctx, data)
}

// QPACKDecoderStreamDecoder 解码对端 decoder stream 反馈指令。
type QPACKDecoderStreamDecoder struct {
	*codec.ByteToMessageDecoder
}

// NewQPACKDecoderStreamDecoder 创建 decoder stream 指令解码器。
func NewQPACKDecoderStreamDecoder() *QPACKDecoderStreamDecoder {
	d := &QPACKDecoderStreamDecoder{}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d
}

// Decode 解码一条完整 decoder stream 指令。
func (d *QPACKDecoderStreamDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	msg, n, ok, err := decodeQPACKDecoderInstruction(in, in.ReaderIndex())
	if err != nil || !ok {
		return nil, err
	}
	if err := in.SkipBytes(n); err != nil {
		return nil, err
	}
	return msg, nil
}

// QPACKDecoderStreamEncoder 编码本端 decoder stream 反馈指令。
type QPACKDecoderStreamEncoder struct{}

// NewQPACKDecoderStreamEncoder 创建 decoder stream 指令编码器。
func NewQPACKDecoderStreamEncoder() *QPACKDecoderStreamEncoder {
	return &QPACKDecoderStreamEncoder{}
}

// Write 编码 decoder stream 指令，其它消息透传。
func (e *QPACKDecoderStreamEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	data, ok, err := appendQPACKDecoderInstruction(nil, msg)
	if err != nil {
		return err
	}
	if !ok {
		return ctx.Write(msg)
	}
	return writeBytes(ctx, data)
}

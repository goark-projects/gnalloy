package http2

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

var clientPrefaceBytes = []byte(ClientPreface)

// PrefaceReceivedEvent 表示 HTTP/2 client connection preface 已完整接收。
type PrefaceReceivedEvent struct{}

// PrefaceDecoder 在连接起始处消费 HTTP/2 client preface，并把剩余字节继续传给后续帧解码器。
type PrefaceDecoder struct {
	*codec.ByteToMessageDecoder
	received bool
}

// NewPrefaceDecoder 创建 HTTP/2 client preface 入站解码器。
func NewPrefaceDecoder() *PrefaceDecoder {
	d := &PrefaceDecoder{}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d
}

// Decode 校验并消费固定 preface；完成后恢复为普通字节透传。
func (d *PrefaceDecoder) Decode(ctx *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if d.received {
		return forwardReadable(in)
	}
	readable := in.ReadableBytes()
	compare := readable
	if compare > len(clientPrefaceBytes) {
		compare = len(clientPrefaceBytes)
	}
	reader := in.ReaderIndex()
	for i := 0; i < compare; i++ {
		b, ok := in.GetByte(reader + i)
		if !ok || b != clientPrefaceBytes[i] {
			return nil, ErrInvalidFrame
		}
	}
	if readable < len(clientPrefaceBytes) {
		return nil, nil
	}
	if err := in.SkipBytes(len(clientPrefaceBytes)); err != nil {
		return nil, err
	}
	d.received = true
	ctx.FireUserEventTriggered(PrefaceReceivedEvent{})
	return forwardReadable(in)
}

// PrefaceEncoder 在 client channel active 时写出 HTTP/2 client connection preface。
type PrefaceEncoder struct{}

// NewPrefaceEncoder 创建 HTTP/2 client preface 出站写入器。
func NewPrefaceEncoder() *PrefaceEncoder {
	return &PrefaceEncoder{}
}

// ChannelActive 使用共享只读常量写出 preface，避免每条连接额外构造字符串。
func (e *PrefaceEncoder) ChannelActive(ctx *channel.HandlerContext) {
	if err := ctx.WriteStaticBytesAndFlush(clientPrefaceBytes); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelActive()
}

func forwardReadable(in *buffer.CompositeByteBuf) (buffer.ByteBuf, error) {
	readable := in.ReadableBytes()
	if readable == 0 {
		return nil, nil
	}
	out, err := in.Slice(in.ReaderIndex(), readable)
	if err != nil {
		return nil, err
	}
	if err := in.SkipBytes(readable); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

package codec

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

// ByteDecoder 是 ByteToMessageDecoder 的实际解码策略。
// Decode 必须在产出消息时推进 reader index，否则会触发 ErrDecoderNoProgress。
type ByteDecoder interface {
	Decode(ctx *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error)
}

// ByteListDecoder 是支持一次解码产出多个消息的字节流解码策略。
type ByteListDecoder interface {
	DecodeBytes(ctx *channel.HandlerContext, in *buffer.CompositeByteBuf, out *MessageList) error
}

// ByteToMessageDecoder 对齐 Netty 的流式解码基类，负责 TCP 半包累积、循环解码和生命周期释放。
type ByteToMessageDecoder struct {
	decoder     ByteDecoder
	listDecoder ByteListDecoder
	cumulation  *buffer.CompositeByteBuf
	out         MessageList
}

func NewByteToMessageDecoder(decoder ByteDecoder) *ByteToMessageDecoder {
	return &ByteToMessageDecoder{decoder: decoder}
}

func NewByteToMessageListDecoder(decoder ByteListDecoder) *ByteToMessageDecoder {
	return &ByteToMessageDecoder{listDecoder: decoder}
}

func (d *ByteToMessageDecoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	in, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if d.decoder == nil && d.listDecoder == nil {
		in.Release()
		ctx.FireExceptionCaught(ErrInvalidDecoder)
		return
	}
	if d.cumulation == nil {
		d.cumulation = buffer.NewCompositeByteBuf()
	}
	d.cumulation.Append(in)
	d.decodeLoop(ctx)
}

func (d *ByteToMessageDecoder) ChannelInactive(ctx *channel.HandlerContext) {
	d.releaseCumulation()
	ctx.FireChannelInactive()
}

func (d *ByteToMessageDecoder) decodeLoop(ctx *channel.HandlerContext) {
	for d.cumulation != nil && d.cumulation.ReadableBytes() > 0 {
		readerBefore := d.cumulation.ReaderIndex()
		readableBefore := d.cumulation.ReadableBytes()

		d.out.Reset()
		if d.listDecoder != nil {
			if err := d.listDecoder.DecodeBytes(ctx, d.cumulation, &d.out); err != nil {
				d.out.ReleaseAll()
				ctx.FireExceptionCaught(err)
				d.releaseCumulation()
				return
			}
		} else {
			out, err := d.decoder.Decode(ctx, d.cumulation)
			if err != nil {
				releaseMessage(out)
				ctx.FireExceptionCaught(err)
				d.releaseCumulation()
				return
			}
			d.out.Add(out)
		}

		progressed := d.cumulation.ReaderIndex() != readerBefore || d.cumulation.ReadableBytes() != readableBefore
		if progressed {
			d.cumulation.DiscardReadComponents()
		}

		if d.out.Len() == 0 {
			if progressed {
				continue
			}
			return
		}
		if !progressed {
			d.out.ReleaseAll()
			ctx.FireExceptionCaught(ErrDecoderNoProgress)
			d.releaseCumulation()
			return
		}
		for i := 0; i < d.out.Len(); i++ {
			ctx.FireChannelRead(d.out.At(i))
		}
		d.out.Reset()
	}
}

func (d *ByteToMessageDecoder) releaseCumulation() {
	if d.cumulation == nil {
		return
	}
	d.cumulation.Release()
	d.cumulation = nil
	d.out.ReleaseAll()
}

func releaseMessage(msg any) {
	if buf, ok := msg.(buffer.ByteBuf); ok && buf != nil {
		buf.Release()
		return
	}
	if releasable, ok := msg.(interface{ Release() }); ok {
		releasable.Release()
	}
}

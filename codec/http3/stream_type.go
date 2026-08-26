package http3

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

// StreamType 是 HTTP/3 单向 QUIC stream 的类型前缀。
type StreamType uint64

const (
	StreamTypeControl      StreamType = 0x00
	StreamTypePush         StreamType = 0x01
	StreamTypeQPACKEncoder StreamType = 0x02
	StreamTypeQPACKDecoder StreamType = 0x03
	// StreamTypeWebTransport 是 WebTransport over HTTP/3 的单向 stream type。
	StreamTypeWebTransport StreamType = 0x54
)

// StreamTypeFrame 表示 QUIC 单向 stream 的首个类型前缀。
type StreamTypeFrame struct {
	Type StreamType
}

// StreamTypeDecoder 从 QUIC 单向 stream 头部解析 HTTP/3 stream type。
type StreamTypeDecoder struct {
	*codec.ByteToMessageDecoder
	seen bool
}

// StreamTypeEncoder 在 QUIC 单向 stream 出站首帧前写入 HTTP/3 stream type。
type StreamTypeEncoder struct {
	streamType StreamType
	wrote      bool
}

// NewStreamTypeDecoder 创建 HTTP/3 stream type decoder。
func NewStreamTypeDecoder() *StreamTypeDecoder {
	d := &StreamTypeDecoder{}
	d.ByteToMessageDecoder = codec.NewByteToMessageListDecoder(d)
	return d
}

// NewStreamTypeEncoder 创建 HTTP/3 stream type encoder。
func NewStreamTypeEncoder(streamType StreamType) *StreamTypeEncoder {
	return &StreamTypeEncoder{streamType: streamType}
}

// DecodeBytes 解码 stream type，并把剩余 frame 字节切片透传给后续 frame decoder。
func (d *StreamTypeDecoder) DecodeBytes(_ *channel.HandlerContext, in *buffer.CompositeByteBuf, out *codec.MessageList) error {
	if !d.seen {
		streamType, n, ok, err := readVarInt(in, in.ReaderIndex())
		if err != nil || !ok {
			return err
		}
		if err := in.SkipBytes(n); err != nil {
			return err
		}
		d.seen = true
		out.Add(StreamTypeFrame{Type: StreamType(streamType)})
	}
	if in.ReadableBytes() == 0 {
		return nil
	}
	payload, err := in.Slice(in.ReaderIndex(), in.ReadableBytes())
	if err != nil {
		return err
	}
	if err := in.SkipBytes(in.ReadableBytes()); err != nil {
		payload.Release()
		return err
	}
	out.Add(payload)
	return nil
}

// Write 确保 stream type 只写一次，后续消息保持原样透传。
func (e *StreamTypeEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	if !e.wrote {
		var prefix []byte
		var err error
		prefix, err = appendVarInt(prefix, uint64(e.streamType))
		if err != nil {
			releaseMessage(msg)
			return err
		}
		if err := writeBytes(ctx, prefix); err != nil {
			releaseMessage(msg)
			return err
		}
		e.wrote = true
	}
	return ctx.Write(msg)
}

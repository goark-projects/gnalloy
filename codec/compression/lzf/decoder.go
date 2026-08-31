package lzf

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	base "goark.dev/gnalloy/codec/compression"
)

// Decoder 按 Netty LZF chunk 格式做流式解码。
type Decoder struct {
	*codec.ByteToMessageDecoder
	maxDecodedBytes int
}

// NewDecoder 创建 LZF 解码器。
func NewDecoder(config Config) (*Decoder, error) {
	limit, err := config.decoderLimit()
	if err != nil {
		return nil, err
	}
	d := &Decoder{maxDecodedBytes: limit}
	d.ByteToMessageDecoder = codec.NewByteToMessageListDecoder(d)
	return d, nil
}

// DecodeBytes 从累积区尽可能切出完整 LZF chunk。
func (d *Decoder) DecodeBytes(ctx *channel.HandlerContext, in *buffer.CompositeByteBuf, out *codec.MessageList) error {
	for {
		if in.ReadableBytes() < rawHeaderLength {
			return nil
		}
		reader := in.ReaderIndex()
		magic, err := in.ReadUnsigned(reader, 2, buffer.BigEndian)
		if err != nil {
			return err
		}
		if magic != uint64('Z')<<8|uint64('V') {
			return ErrCorruptFrame
		}
		blockType, ok := in.GetByte(reader + 2)
		if !ok {
			return buffer.ErrNotEnoughBytes
		}
		if blockType != blockTypeRaw && blockType != blockTypeCompressed {
			return ErrCorruptFrame
		}
		chunkLength64, err := in.ReadUnsigned(reader+3, 2, buffer.BigEndian)
		if err != nil {
			return err
		}
		chunkLength := int(chunkLength64)
		headerLength := rawHeaderLength
		originalLength := chunkLength
		if blockType == blockTypeCompressed {
			headerLength = compressedHeaderLength
			if in.ReadableBytes() < headerLength {
				return nil
			}
			originalLength64, err := in.ReadUnsigned(reader+5, 2, buffer.BigEndian)
			if err != nil {
				return err
			}
			originalLength = int(originalLength64)
		}
		if originalLength > d.maxDecodedBytes {
			return base.ErrDecodedTooLong
		}
		frameLength := headerLength + chunkLength
		if in.ReadableBytes() < frameLength {
			return nil
		}
		msg, err := d.decodeChunk(ctx.Channel().Allocator(), in, reader+headerLength, chunkLength, originalLength, blockType)
		if err != nil {
			return err
		}
		if err := in.SkipBytes(frameLength); err != nil {
			if msg != nil {
				msg.Release()
			}
			return err
		}
		if msg != nil && msg.ReadableBytes() > 0 {
			out.Add(msg)
		} else if msg != nil {
			msg.Release()
		}
	}
}

func (d *Decoder) decodeChunk(alloc buffer.Allocator, in *buffer.CompositeByteBuf, payloadIndex int, chunkLength int, originalLength int, blockType byte) (buffer.ByteBuf, error) {
	if blockType == blockTypeRaw {
		return in.Slice(payloadIndex, chunkLength)
	}
	if originalLength == 0 {
		return buffer.NewHeapBuffer(0), nil
	}
	payload, release, err := sliceBytes(in, payloadIndex, chunkLength)
	if err != nil {
		return nil, err
	}
	defer release()
	out, err := alloc.Acquire(originalLength)
	if err != nil {
		return nil, err
	}
	view := out.WritableBytesView()
	if len(view) < originalLength {
		out.Release()
		return nil, buffer.ErrNoWritableBytes
	}
	n, err := decompressBlock(view[:originalLength], payload)
	if err != nil {
		out.Release()
		return nil, err
	}
	if n != originalLength {
		out.Release()
		return nil, ErrCorruptFrame
	}
	if err := out.AdvanceWriter(n); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

func sliceBytes(in *buffer.CompositeByteBuf, index int, length int) ([]byte, func(), error) {
	frame, err := in.Slice(index, length)
	if err != nil {
		return nil, nil, err
	}
	if data, ok := buffer.ContiguousReadableBytes(frame); ok {
		return data, func() { frame.Release() }, nil
	}
	data := make([]byte, length)
	buffer.CopyReadableBytes(data, frame)
	frame.Release()
	return data, func() {}, nil
}

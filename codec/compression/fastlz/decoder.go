package fastlz

import (
	"hash/adler32"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	base "goark.dev/gnalloy/codec/compression"
)

// Decoder 按 Netty FastLZ frame 格式做流式解码。
type Decoder struct {
	*codec.ByteToMessageDecoder
	checksum        bool
	maxDecodedBytes int
}

// NewDecoder 创建 FastLZ 解码器。
func NewDecoder(config Config) (*Decoder, error) {
	limit, err := config.decoderLimit()
	if err != nil {
		return nil, err
	}
	d := &Decoder{checksum: config.Checksum, maxDecodedBytes: limit}
	d.ByteToMessageDecoder = codec.NewByteToMessageListDecoder(d)
	return d, nil
}

// DecodeBytes 从累积区尽可能切出完整 FastLZ frame。
func (d *Decoder) DecodeBytes(ctx *channel.HandlerContext, in *buffer.CompositeByteBuf, out *codec.MessageList) error {
	for {
		if in.ReadableBytes() < minHeaderLength {
			return nil
		}
		reader := in.ReaderIndex()
		magic, err := in.ReadUnsigned(reader, 3, buffer.BigEndian)
		if err != nil {
			return err
		}
		if magic != uint64(magicNumber) {
			return ErrCorruptFrame
		}
		options, ok := in.GetByte(reader + 3)
		if !ok {
			return buffer.ErrNotEnoughBytes
		}
		compressed := options&optionCompressed != 0
		hasChecksum := options&optionChecksum != 0
		headerLength := 4
		checksumValue := uint32(0)
		if hasChecksum {
			if in.ReadableBytes() < headerLength+4+2 {
				return nil
			}
			rawChecksum, err := in.ReadUnsigned(reader+headerLength, 4, buffer.BigEndian)
			if err != nil {
				return err
			}
			checksumValue = uint32(rawChecksum)
			headerLength += 4
		}
		requiredHeader := headerLength + 2
		if compressed {
			requiredHeader += 2
		}
		if in.ReadableBytes() < requiredHeader {
			return nil
		}
		chunkLength64, err := in.ReadUnsigned(reader+headerLength, 2, buffer.BigEndian)
		if err != nil {
			return err
		}
		chunkLength := int(chunkLength64)
		headerLength += 2
		originalLength := chunkLength
		if compressed {
			originalLength64, err := in.ReadUnsigned(reader+headerLength, 2, buffer.BigEndian)
			if err != nil {
				return err
			}
			originalLength = int(originalLength64)
			headerLength += 2
		}
		if originalLength > d.maxDecodedBytes {
			return base.ErrDecodedTooLong
		}
		frameLength := headerLength + chunkLength
		if in.ReadableBytes() < frameLength {
			return nil
		}
		msg, err := d.decodePayload(ctx.Channel().Allocator(), in, reader+headerLength, chunkLength, originalLength, compressed)
		if err != nil {
			return err
		}
		if hasChecksum && d.checksum {
			if adler32.Checksum(msg.Bytes()) != checksumValue {
				msg.Release()
				return ErrChecksumMismatch
			}
		}
		if err := in.SkipBytes(frameLength); err != nil {
			msg.Release()
			return err
		}
		if msg.ReadableBytes() > 0 {
			out.Add(msg)
		} else {
			msg.Release()
		}
	}
}

func (d *Decoder) decodePayload(alloc buffer.Allocator, in *buffer.CompositeByteBuf, index int, chunkLength int, originalLength int, compressed bool) (buffer.ByteBuf, error) {
	if !compressed {
		return in.Slice(index, chunkLength)
	}
	payload, release, err := sliceBytes(in, index, chunkLength)
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

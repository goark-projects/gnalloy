package snappy

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/compression/internal/stream"

	nativesnappy "github.com/golang/snappy"
)

const (
	chunkTypeCompressed   = 0x00
	chunkTypeUncompressed = 0x01
	chunkTypeStreamID     = 0xff

	defaultFrameBlockSize = 32 * 1024
	maxFrameBlockSize     = 64 * 1024
	minCompressibleBytes  = 18
	chunkHeaderSize       = 4
	checksumSize          = 4
)

var streamIdentifierChunk = []byte{chunkTypeStreamID, 6, 0, 0, 's', 'N', 'a', 'P', 'p', 'Y'}

// FrameEncoderConfig 描述 Snappy framed 编码参数。
type FrameEncoderConfig struct {
	// BlockSize 是单个 Snappy block 的最大未压缩字节数，0 使用 32KiB。
	BlockSize int
}

// FrameEncoder 输出 Netty SnappyFrameEncoder/SnappyFramedEncoder 兼容帧。
type FrameEncoder struct {
	blockSize int
	started   bool
}

// NewFrameEncoder 创建 Netty framed Snappy 编码器。
func NewFrameEncoder() *FrameEncoder {
	return NewFrameEncoderWithConfig(FrameEncoderConfig{})
}

// NewFramedEncoder 创建 Netty framed Snappy 编码器。
func NewFramedEncoder() *FrameEncoder {
	return NewFrameEncoder()
}

// NewFrameEncoderWithConfig 创建可配置 block 大小的 Netty framed Snappy 编码器。
func NewFrameEncoderWithConfig(cfg FrameEncoderConfig) *FrameEncoder {
	blockSize := cfg.BlockSize
	if blockSize <= 0 {
		blockSize = defaultFrameBlockSize
	}
	if blockSize > maxFrameBlockSize {
		blockSize = maxFrameBlockSize
	}
	return &FrameEncoder{blockSize: blockSize}
}

// Write 将 ByteBuf 编码为 Snappy framed chunk 序列。
func (e *FrameEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return ctx.Write(msg)
	}
	out, err := e.encode(ctx, buf)
	buf.Release()
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := ctx.Write(out); err != nil {
		out.Release()
		return err
	}
	return nil
}

func (e *FrameEncoder) encode(ctx *channel.HandlerContext, src buffer.ByteBuf) (buffer.ByteBuf, error) {
	data := src.Bytes()
	if len(data) == 0 {
		return nil, nil
	}
	estimate := len(data) + len(data)/6 + len(streamIdentifierChunk) + chunkHeaderSize + checksumSize
	encoded := make([]byte, 0, estimate)
	if !e.started {
		encoded = append(encoded, streamIdentifierChunk...)
		e.started = true
	}
	for len(data) > 0 {
		n := e.blockSize
		if n > len(data) {
			n = len(data)
		}
		part := data[:n]
		if len(part) < minCompressibleBytes {
			encoded = appendUncompressedChunk(encoded, part)
		} else {
			encoded = appendCompressedChunk(encoded, part)
		}
		data = data[n:]
	}
	return stream.ByteBufFromBytes(ctx.Channel().Allocator(), encoded)
}

func appendUncompressedChunk(dst []byte, data []byte) []byte {
	dst = append(dst, chunkTypeUncompressed)
	dst = appendLittleMedium(dst, len(data)+checksumSize)
	dst = appendChecksum(dst, data)
	return append(dst, data...)
}

func appendCompressedChunk(dst []byte, data []byte) []byte {
	compressed := nativesnappy.Encode(nil, data)
	dst = append(dst, chunkTypeCompressed)
	dst = appendLittleMedium(dst, len(compressed)+checksumSize)
	dst = appendChecksum(dst, data)
	return append(dst, compressed...)
}

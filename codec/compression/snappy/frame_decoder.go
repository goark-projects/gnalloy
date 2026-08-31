package snappy

import (
	"bytes"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	base "goark.dev/gnalloy/codec/compression"
	"goark.dev/gnalloy/codec/compression/internal/stream"

	nativesnappy "github.com/golang/snappy"
)

// FrameDecoderConfig 描述 Snappy framed 解码边界。
type FrameDecoderConfig struct {
	// MaxDecodedBytes 限制单个解码 chunk 的最大未压缩字节数，0 表示不额外限制。
	MaxDecodedBytes int
	// ValidateChecksums 控制是否验证 masked CRC32C。
	ValidateChecksums bool
}

// FrameDecoder 解析 Netty SnappyFrameDecoder/SnappyFramedDecoder 兼容帧。
type FrameDecoder struct {
	*codec.ByteToMessageDecoder
	cfg           FrameDecoderConfig
	started       bool
	skipRemaining int
}

// NewFrameDecoder 创建 Netty framed Snappy 解码器。
func NewFrameDecoder(cfg FrameDecoderConfig) *FrameDecoder {
	d := &FrameDecoder{cfg: cfg}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d
}

// NewFramedDecoder 创建 Netty framed Snappy 解码器。
func NewFramedDecoder(cfg FrameDecoderConfig) *FrameDecoder {
	return NewFrameDecoder(cfg)
}

// Decode 解析一个完整 Snappy framed chunk；不完整时返回 nil。
func (d *FrameDecoder) Decode(ctx *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if d.skipRemaining > 0 {
		d.skip(in)
		return nil, nil
	}
	if in.ReadableBytes() < chunkHeaderSize {
		return nil, nil
	}
	reader := in.ReaderIndex()
	chunkType, _ := in.GetByte(reader)
	length, err := readFrameLength(in, reader+1)
	if err != nil {
		return nil, err
	}
	switch {
	case chunkType == chunkTypeStreamID:
		return d.readStreamIdentifier(in, reader, length)
	case isSkippableChunk(chunkType):
		return d.readSkippable(in, length)
	case chunkType == chunkTypeUncompressed:
		return d.readUncompressed(in, reader, length)
	case chunkType == chunkTypeCompressed:
		return d.readCompressed(ctx, in, reader, length)
	default:
		return nil, ErrReservedChunkType
	}
}

func (d *FrameDecoder) readStreamIdentifier(in *buffer.CompositeByteBuf, reader int, length int) (any, error) {
	if length != len(streamIdentifierChunk)-chunkHeaderSize {
		return nil, ErrInvalidFrame
	}
	if in.ReadableBytes() < len(streamIdentifierChunk) {
		return nil, nil
	}
	chunk, err := in.Slice(reader, len(streamIdentifierChunk))
	if err != nil {
		return nil, err
	}
	ok := bytes.Equal(chunk.Bytes(), streamIdentifierChunk)
	chunk.Release()
	if !ok {
		return nil, ErrInvalidFrame
	}
	if err := in.SkipBytes(len(streamIdentifierChunk)); err != nil {
		return nil, err
	}
	d.started = true
	return nil, nil
}

func (d *FrameDecoder) readSkippable(in *buffer.CompositeByteBuf, length int) (any, error) {
	if err := d.ensureStarted(); err != nil {
		return nil, err
	}
	if err := in.SkipBytes(chunkHeaderSize); err != nil {
		return nil, err
	}
	d.skipRemaining = length
	d.skip(in)
	return nil, nil
}

func (d *FrameDecoder) readUncompressed(in *buffer.CompositeByteBuf, reader int, length int) (any, error) {
	if err := d.ensureStarted(); err != nil {
		return nil, err
	}
	if length < checksumSize {
		return nil, ErrInvalidFrame
	}
	total := chunkHeaderSize + length
	if in.ReadableBytes() < total {
		return nil, nil
	}
	payloadLen := length - checksumSize
	if err := d.ensureDecodedLimit(payloadLen); err != nil {
		return nil, err
	}
	checksum, err := readFrameChecksum(in, reader+chunkHeaderSize)
	if err != nil {
		return nil, err
	}
	payload, err := in.Slice(reader+chunkHeaderSize+checksumSize, payloadLen)
	if err != nil {
		return nil, err
	}
	if d.cfg.ValidateChecksums && maskedChecksum(payload.Bytes()) != checksum {
		payload.Release()
		return nil, ErrInvalidChecksum
	}
	if err := in.SkipBytes(total); err != nil {
		payload.Release()
		return nil, err
	}
	return payload, nil
}

func (d *FrameDecoder) readCompressed(ctx *channel.HandlerContext, in *buffer.CompositeByteBuf, reader int, length int) (any, error) {
	if err := d.ensureStarted(); err != nil {
		return nil, err
	}
	if length <= checksumSize {
		return nil, ErrInvalidFrame
	}
	total := chunkHeaderSize + length
	if in.ReadableBytes() < total {
		return nil, nil
	}
	checksum, err := readFrameChecksum(in, reader+chunkHeaderSize)
	if err != nil {
		return nil, err
	}
	compressed, err := in.Slice(reader+chunkHeaderSize+checksumSize, length-checksumSize)
	if err != nil {
		return nil, err
	}
	decodedLen, err := nativesnappy.DecodedLen(compressed.Bytes())
	if err != nil {
		compressed.Release()
		return nil, err
	}
	if err := d.ensureDecodedLimit(decodedLen); err != nil {
		compressed.Release()
		return nil, err
	}
	decoded, err := nativesnappy.Decode(nil, compressed.Bytes())
	compressed.Release()
	if err != nil {
		return nil, err
	}
	if d.cfg.ValidateChecksums && maskedChecksum(decoded) != checksum {
		return nil, ErrInvalidChecksum
	}
	if err := in.SkipBytes(total); err != nil {
		return nil, err
	}
	return stream.ByteBufFromBytes(ctx.Channel().Allocator(), decoded)
}

func (d *FrameDecoder) ensureStarted() error {
	if !d.started {
		return ErrInvalidFrame
	}
	return nil
}

func (d *FrameDecoder) ensureDecodedLimit(size int) error {
	if d.cfg.MaxDecodedBytes > 0 && size > d.cfg.MaxDecodedBytes {
		return base.ErrDecodedTooLong
	}
	return nil
}

func (d *FrameDecoder) skip(in *buffer.CompositeByteBuf) {
	if d.skipRemaining == 0 || in.ReadableBytes() == 0 {
		return
	}
	n := d.skipRemaining
	if n > in.ReadableBytes() {
		n = in.ReadableBytes()
	}
	if err := in.SkipBytes(n); err != nil {
		return
	}
	d.skipRemaining -= n
}

func readFrameLength(in *buffer.CompositeByteBuf, index int) (int, error) {
	if in.WriterIndex()-index < 3 {
		return 0, ErrInvalidFrame
	}
	var raw [3]byte
	for i := range raw {
		b, ok := in.GetByte(index + i)
		if !ok {
			return 0, ErrInvalidFrame
		}
		raw[i] = b
	}
	return readLittleMedium(raw[:]), nil
}

func readFrameChecksum(in *buffer.CompositeByteBuf, index int) (uint32, error) {
	if in.WriterIndex()-index < checksumSize {
		return 0, ErrInvalidFrame
	}
	var raw [4]byte
	for i := range raw {
		b, ok := in.GetByte(index + i)
		if !ok {
			return 0, ErrInvalidFrame
		}
		raw[i] = b
	}
	return readLittleUint32(raw[:]), nil
}

func isSkippableChunk(chunkType byte) bool {
	return chunkType >= 0x80 && chunkType <= 0xfe
}

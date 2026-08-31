package lzf

import (
	"sync"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	base "goark.dev/gnalloy/codec/compression"
)

// Encoder 把 ByteBuf 编码为 Netty 兼容的 LZF chunk 流。
type Encoder struct {
	threshold int
	hashPool  sync.Pool
}

// NewEncoder 创建 LZF 编码器。
func NewEncoder(config Config) (*Encoder, error) {
	threshold, err := config.encoderThreshold()
	if err != nil {
		return nil, err
	}
	return &Encoder{threshold: threshold}, nil
}

// Write 压缩 ByteBuf 并把 LZF chunk 写出。
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
	if alloc == nil {
		return nil, base.ErrInvalidConfig
	}
	length := src.ReadableBytes()
	out, err := alloc.Acquire(estimateFrameSize(length))
	if err != nil {
		return nil, err
	}
	payload, release := readableBytes(src)
	defer release()
	if err := e.encodeBytes(out, payload); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

func (e *Encoder) encodeBytes(out buffer.ByteBuf, payload []byte) error {
	for len(payload) > 0 {
		chunkLen := len(payload)
		if chunkLen > maxChunkLength {
			chunkLen = maxChunkLength
		}
		chunk := payload[:chunkLen]
		if chunkLen >= e.threshold {
			hashes := e.acquireHashes()
			tmp := make([]byte, maxCompressedLength(chunkLen))
			compressed, ok := compressBlock(tmp, chunk, hashes)
			e.releaseHashes(hashes)
			if ok && compressed+compressedHeaderLength < rawHeaderLength+chunkLen {
				if err := writeCompressedChunk(out, tmp[:compressed], chunkLen); err != nil {
					return err
				}
				payload = payload[chunkLen:]
				continue
			}
		}
		if err := writeRawChunk(out, chunk); err != nil {
			return err
		}
		payload = payload[chunkLen:]
	}
	return nil
}

func (e *Encoder) acquireHashes() []int {
	if value := e.hashPool.Get(); value != nil {
		hashes := value.([]int)
		clear(hashes)
		return hashes
	}
	return make([]int, lzfHashSize)
}

func (e *Encoder) releaseHashes(hashes []int) {
	if len(hashes) == lzfHashSize {
		e.hashPool.Put(hashes)
	}
}

func readableBytes(src buffer.ByteBuf) ([]byte, func()) {
	if data, ok := buffer.ContiguousReadableBytes(src); ok {
		return data, func() {}
	}
	data := make([]byte, src.ReadableBytes())
	buffer.CopyReadableBytes(data, src)
	return data, func() {}
}

func estimateFrameSize(length int) int {
	if length == 0 {
		return 1
	}
	chunks := (length + maxChunkLength - 1) / maxChunkLength
	return length + chunks*rawHeaderLength
}

func maxCompressedLength(length int) int {
	return length + length/16 + 64
}

func writeRawChunk(out buffer.ByteBuf, payload []byte) error {
	if _, err := out.WriteBytes([]byte{'Z', 'V', blockTypeRaw, byte(len(payload) >> 8), byte(len(payload))}); err != nil {
		return err
	}
	_, err := out.WriteBytes(payload)
	return err
}

func writeCompressedChunk(out buffer.ByteBuf, payload []byte, originalLength int) error {
	header := []byte{
		'Z', 'V', blockTypeCompressed,
		byte(len(payload) >> 8), byte(len(payload)),
		byte(originalLength >> 8), byte(originalLength),
	}
	if _, err := out.WriteBytes(header); err != nil {
		return err
	}
	_, err := out.WriteBytes(payload)
	return err
}

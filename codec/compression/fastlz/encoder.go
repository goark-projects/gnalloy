package fastlz

import (
	"hash/adler32"
	"sync"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	base "goark.dev/gnalloy/codec/compression"
)

// Encoder 把 ByteBuf 编码为 Netty 兼容的 FastLZ frame。
type Encoder struct {
	level    Level
	checksum bool
	hashPool sync.Pool
}

// NewEncoder 创建 FastLZ 编码器。
func NewEncoder(config Config) (*Encoder, error) {
	level, err := config.encoderLevel()
	if err != nil {
		return nil, err
	}
	return &Encoder{level: level, checksum: config.Checksum}, nil
}

// Write 压缩 ByteBuf 并把 FastLZ frame 写出。
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
	payload := readableBytes(src)
	out, err := alloc.Acquire(estimateFrameSize(len(payload), e.checksum))
	if err != nil {
		return nil, err
	}
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
		if chunkLen >= minCompressBytes {
			hashes := e.acquireHashes()
			tmp := make([]byte, maxOutputLength(chunkLen))
			compressed, ok := compressBlock(tmp, chunk, chooseLevel(e.level, chunkLen), hashes)
			e.releaseHashes(hashes)
			if ok && compressed < chunkLen {
				if err := writeFrame(out, tmp[:compressed], chunkLen, true, e.checksum, chunk); err != nil {
					return err
				}
				payload = payload[chunkLen:]
				continue
			}
		}
		if err := writeFrame(out, chunk, chunkLen, false, e.checksum, chunk); err != nil {
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
	return make([]int, fastHashSize)
}

func (e *Encoder) releaseHashes(hashes []int) {
	if len(hashes) == fastHashSize {
		e.hashPool.Put(hashes)
	}
}

func readableBytes(src buffer.ByteBuf) []byte {
	if data, ok := buffer.ContiguousReadableBytes(src); ok {
		return data
	}
	data := make([]byte, src.ReadableBytes())
	buffer.CopyReadableBytes(data, src)
	return data
}

func chooseLevel(level Level, length int) Level {
	if level != LevelAuto {
		return level
	}
	if length < fastLevel2MinBytes {
		return Level1
	}
	return Level2
}

func estimateFrameSize(length int, checksum bool) int {
	if length == 0 {
		return 1
	}
	chunks := (length + maxChunkLength - 1) / maxChunkLength
	header := 8
	if checksum {
		header += 4
	}
	return length + chunks*header
}

func maxOutputLength(length int) int {
	out := length + length/16
	if out < 66 {
		out = 66
	}
	return out
}

func writeFrame(out buffer.ByteBuf, payload []byte, originalLength int, compressed bool, checksum bool, original []byte) error {
	header := []byte{'F', 'L', 'Z', 0}
	if compressed {
		header[3] |= optionCompressed
	}
	if checksum {
		header[3] |= optionChecksum
	}
	if _, err := out.WriteBytes(header); err != nil {
		return err
	}
	if checksum {
		sum := adler32.Checksum(original)
		if _, err := out.WriteBytes([]byte{byte(sum >> 24), byte(sum >> 16), byte(sum >> 8), byte(sum)}); err != nil {
			return err
		}
	}
	if compressed {
		if _, err := out.WriteBytes([]byte{byte(len(payload) >> 8), byte(len(payload)), byte(originalLength >> 8), byte(originalLength)}); err != nil {
			return err
		}
	} else if _, err := out.WriteBytes([]byte{byte(originalLength >> 8), byte(originalLength)}); err != nil {
		return err
	}
	_, err := out.WriteBytes(payload)
	return err
}

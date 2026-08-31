package compression

import (
	"bytes"
	"compress/flate"
	"io"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/codec"
)

const DefaultStreamChunkSize = 16 * 1024

type flushWriteCloser interface {
	io.WriteCloser
	Flush() error
}

// ChunkedEncoderConfig 描述流式 ChunkedInput 压缩参数。
type ChunkedEncoderConfig struct {
	Format    Format
	Level     int
	ChunkSize int
}

// CompressingChunkedInput 将上游 ChunkedInput 转换为压缩后的字节分片。
type CompressingChunkedInput struct {
	input        codec.ChunkedInput
	format       Format
	level        int
	chunkSize    int
	dst          bytes.Buffer
	writer       flushWriteCloser
	sourceDone   bool
	writerClosed bool
	closed       bool
}

// NewCompressingChunkedInput 创建 pull-based 流式压缩输入。
func NewCompressingChunkedInput(input codec.ChunkedInput, cfg ChunkedEncoderConfig) (*CompressingChunkedInput, error) {
	if input == nil || !validFormat(cfg.Format) {
		if input != nil {
			_ = input.Close()
		}
		return nil, ErrInvalidConfig
	}
	level := cfg.Level
	if level == 0 {
		level = flate.DefaultCompression
	}
	if !validLevel(level) {
		_ = input.Close()
		return nil, ErrInvalidConfig
	}
	chunkSize := cfg.ChunkSize
	if chunkSize <= 0 {
		chunkSize = DefaultStreamChunkSize
	}
	return &CompressingChunkedInput{input: input, format: cfg.Format, level: level, chunkSize: chunkSize}, nil
}

// ReadChunk 读取下一段压缩后字节。
func (i *CompressingChunkedInput) ReadChunk(alloc buffer.Allocator) (buffer.ByteBuf, bool, error) {
	if i == nil || i.closed {
		return nil, true, nil
	}
	if alloc == nil {
		return nil, false, ErrInvalidConfig
	}
	if err := i.ensureWriter(); err != nil {
		return nil, false, err
	}
	for {
		if i.dst.Len() > 0 {
			chunk, err := i.drain(alloc)
			return chunk, i.writerClosed && i.dst.Len() == 0, err
		}
		if i.writerClosed {
			return nil, true, nil
		}
		if err := i.pullSource(alloc); err != nil {
			return nil, false, err
		}
	}
}

// Close 关闭压缩状态和上游输入。
func (i *CompressingChunkedInput) Close() error {
	if i == nil || i.closed {
		return nil
	}
	i.closed = true
	var err error
	if i.writer != nil && !i.writerClosed {
		err = i.writer.Close()
		i.writerClosed = true
	}
	if i.input != nil {
		if closeErr := i.input.Close(); err == nil {
			err = closeErr
		}
		i.input = nil
	}
	i.dst.Reset()
	return err
}

func (i *CompressingChunkedInput) ensureWriter() error {
	if i.writer != nil {
		return nil
	}
	writer, err := newCompressionWriter(i.format, i.level, &i.dst)
	if err != nil {
		return err
	}
	flusher, ok := writer.(flushWriteCloser)
	if !ok {
		_ = writer.Close()
		return ErrInvalidConfig
	}
	i.writer = flusher
	return nil
}

func (i *CompressingChunkedInput) pullSource(alloc buffer.Allocator) error {
	chunk, done, err := i.input.ReadChunk(alloc)
	if err != nil {
		if chunk != nil {
			chunk.Release()
		}
		return err
	}
	if chunk != nil {
		if err := writeByteBufTo(i.writer, chunk); err != nil {
			chunk.Release()
			return err
		}
		chunk.Release()
	}
	if err := i.writer.Flush(); err != nil {
		return err
	}
	if done {
		i.sourceDone = true
		if err := i.writer.Close(); err != nil {
			return err
		}
		i.writerClosed = true
	}
	return nil
}

func (i *CompressingChunkedInput) drain(alloc buffer.Allocator) (buffer.ByteBuf, error) {
	n := i.chunkSize
	if n > i.dst.Len() {
		n = i.dst.Len()
	}
	out, err := alloc.Acquire(n)
	if err != nil {
		return nil, err
	}
	if _, err := out.WriteBytes(i.dst.Next(n)); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

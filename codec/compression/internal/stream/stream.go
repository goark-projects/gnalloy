package stream

import (
	"bytes"
	"io"

	"goark.dev/gnalloy/buffer"
	base "goark.dev/gnalloy/codec/compression"
)

// DecodeAll 按最大解压字节数读取 reader 并写入 ByteBuf。
func DecodeAll(alloc buffer.Allocator, reader io.Reader, maxDecodedBytes int) (buffer.ByteBuf, error) {
	if alloc == nil || maxDecodedBytes < 0 {
		return nil, base.ErrInvalidConfig
	}
	if maxDecodedBytes == 0 {
		maxDecodedBytes = base.DefaultMaxDecodedBytes
	}
	limited := io.LimitReader(reader, int64(maxDecodedBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > maxDecodedBytes {
		return nil, base.ErrDecodedTooLong
	}
	return ByteBufFromBytes(alloc, data)
}

// ByteBufFromBytes 把普通字节复制进 allocator 管理的 ByteBuf。
func ByteBufFromBytes(alloc buffer.Allocator, data []byte) (buffer.ByteBuf, error) {
	if alloc == nil {
		return nil, base.ErrInvalidConfig
	}
	size := len(data)
	if size == 0 {
		size = 1
	}
	out, err := alloc.Acquire(size)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return out, nil
	}
	if _, err := out.WriteBytes(data); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

// NewByteBufReader 返回可按 ByteBuf readable slices 顺序读取的 io.Reader。
func NewByteBufReader(src buffer.ByteBuf) io.Reader {
	if data, ok := buffer.ContiguousReadableBytes(src); ok {
		return bytes.NewReader(data)
	}
	reader := &byteBufReader{}
	reader.slices = src.ReadableSlices(reader.stack[:0])
	return reader
}

// WriteByteBuf 把 ByteBuf readable slices 顺序写入 writer，不复制 payload。
func WriteByteBuf(writer io.Writer, src buffer.ByteBuf) error {
	if src == nil || src.ReadableBytes() == 0 {
		return nil
	}
	if data, ok := buffer.ContiguousReadableBytes(src); ok {
		_, err := writer.Write(data)
		return err
	}
	var stack [8][]byte
	slices := src.ReadableSlices(stack[:0])
	if len(slices) == 0 {
		return buffer.ErrInvalidIndex
	}
	for _, part := range slices {
		if len(part) == 0 {
			continue
		}
		if _, err := writer.Write(part); err != nil {
			return err
		}
	}
	return nil
}

type byteBufReader struct {
	stack  [8][]byte
	slices [][]byte
	index  int
	offset int
}

func (r *byteBufReader) Read(dst []byte) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	written := 0
	for written < len(dst) && r.index < len(r.slices) {
		current := r.slices[r.index]
		if r.offset >= len(current) {
			r.index++
			r.offset = 0
			continue
		}
		n := copy(dst[written:], current[r.offset:])
		written += n
		r.offset += n
	}
	if written == 0 {
		return 0, io.EOF
	}
	return written, nil
}

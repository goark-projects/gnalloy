package compression

import (
	"bytes"
	"io"

	"goark.dev/gnalloy/buffer"
)

func bytesReader(src []byte) io.Reader {
	return bytes.NewReader(src)
}

func newByteBufReadSource(src buffer.ByteBuf) io.Reader {
	if data, ok := buffer.ContiguousReadableBytes(src); ok {
		return bytes.NewReader(data)
	}
	reader := &byteBufReadSource{}
	reader.slices = src.ReadableSlices(reader.stack[:0])
	return reader
}

func writeByteBufTo(dst io.Writer, src buffer.ByteBuf) error {
	if src == nil || src.ReadableBytes() == 0 {
		return nil
	}
	if data, ok := buffer.ContiguousReadableBytes(src); ok {
		_, err := dst.Write(data)
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
		if _, err := dst.Write(part); err != nil {
			return err
		}
	}
	return nil
}

type byteBufReadSource struct {
	stack  [8][]byte
	slices [][]byte
	index  int
	offset int
}

func (r *byteBufReadSource) Read(dst []byte) (int, error) {
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

package deflate

import (
	"bytes"
	"compress/flate"
	"errors"
	"io"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/codec"
)

var syncFlushTail = []byte{0x00, 0x00, 0xff, 0xff}

func compressMessage(src []byte, level int) ([]byte, error) {
	var dst bytes.Buffer
	writer, err := flate.NewWriter(&dst, level)
	if err != nil {
		return nil, err
	}
	if len(src) > 0 {
		if _, err := writer.Write(src); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Flush(); err != nil {
		_ = writer.Close()
		return nil, err
	}
	compressed := append([]byte(nil), dst.Bytes()...)
	_ = writer.Close()
	return trimSyncFlushTail(compressed), nil
}

func decompressMessage(src []byte, maxMessageBytes int) ([]byte, error) {
	payload := make([]byte, 0, len(src)+len(syncFlushTail))
	payload = append(payload, src...)
	payload = append(payload, syncFlushTail...)
	reader := flate.NewReader(bytes.NewReader(payload))
	defer reader.Close()
	limited := io.LimitReader(reader, int64(maxMessageBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	if len(data) > maxMessageBytes {
		return nil, codec.ErrFrameTooLong
	}
	return data, nil
}

func trimSyncFlushTail(data []byte) []byte {
	if len(data) < len(syncFlushTail) {
		return data
	}
	start := len(data) - len(syncFlushTail)
	if bytes.Equal(data[start:], syncFlushTail) {
		return data[:start]
	}
	return data
}

func byteBufFromBytes(alloc buffer.Allocator, data []byte) (buffer.ByteBuf, error) {
	size := len(data)
	if size == 0 {
		size = 1
	}
	out, err := alloc.Acquire(size)
	if err != nil {
		return nil, err
	}
	if len(data) > 0 {
		if _, err := out.WriteBytes(data); err != nil {
			out.Release()
			return nil, err
		}
	}
	return out, nil
}

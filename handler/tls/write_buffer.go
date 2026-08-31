package tls

import (
	"io"

	"goark.dev/gnalloy/buffer"
)

func writeTLSBuffer(conn interface{ Write([]byte) (int, error) }, buf buffer.ByteBuf) error {
	if conn == nil || buf == nil || buf.ReadableBytes() == 0 {
		return nil
	}
	if data, ok := buffer.ContiguousReadableBytes(buf); ok {
		return writeAllTLS(conn, data)
	}
	var stack [8][]byte
	for _, part := range buf.ReadableSlices(stack[:0]) {
		if err := writeAllTLS(conn, part); err != nil {
			return err
		}
	}
	return nil
}

func writeAllTLS(conn interface{ Write([]byte) (int, error) }, data []byte) error {
	for len(data) > 0 {
		n, err := conn.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

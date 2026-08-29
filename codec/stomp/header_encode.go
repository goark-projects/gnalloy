package stomp

import (
	"strconv"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/codec"
)

func encodedHeaderSize(frame Frame, bodyLen int, hasContentLength bool) int {
	size := len(frame.Command) + 2
	for _, header := range frame.Headers {
		size += escapedLen(header.Name) + 1 + escapedLen(header.Value) + 1
	}
	if frame.Body != nil && !hasContentLength {
		size += len("content-length:") + decimalLen(bodyLen) + 1
	}
	return size
}

func writeHeader(out buffer.ByteBuf, frame Frame, bodyLen int, hasContentLength bool) error {
	size := encodedHeaderSize(frame, bodyLen, hasContentLength)
	view := out.WritableBytesView()
	if len(view) < size {
		return writeHeaderFallback(out, frame, bodyLen, hasContentLength)
	}
	n := appendHeader(view[:0], frame, bodyLen, hasContentLength)
	if n != size {
		return codec.ErrInvalidFrameLength
	}
	return out.AdvanceWriter(n)
}

func writeHeaderFallback(out buffer.ByteBuf, frame Frame, bodyLen int, hasContentLength bool) error {
	var tmp [32]byte
	if _, err := out.WriteBytes([]byte(frame.Command)); err != nil {
		return err
	}
	if _, err := out.WriteBytes([]byte{'\n'}); err != nil {
		return err
	}
	for _, header := range frame.Headers {
		if err := writeEscaped(out, header.Name, tmp[:]); err != nil {
			return err
		}
		if _, err := out.WriteBytes([]byte{':'}); err != nil {
			return err
		}
		if err := writeEscaped(out, header.Value, tmp[:]); err != nil {
			return err
		}
		if _, err := out.WriteBytes([]byte{'\n'}); err != nil {
			return err
		}
	}
	if frame.Body != nil && !hasContentLength {
		if _, err := out.WriteBytes([]byte("content-length:")); err != nil {
			return err
		}
		n := strconv.AppendInt(tmp[:0], int64(bodyLen), 10)
		if _, err := out.WriteBytes(n); err != nil {
			return err
		}
		if _, err := out.WriteBytes([]byte{'\n'}); err != nil {
			return err
		}
	}
	_, err := out.WriteBytes([]byte{'\n'})
	return err
}

func appendHeader(dst []byte, frame Frame, bodyLen int, hasContentLength bool) int {
	start := len(dst)
	dst = append(dst, string(frame.Command)...)
	dst = append(dst, '\n')
	for _, header := range frame.Headers {
		dst = appendEscaped(dst, header.Name)
		dst = append(dst, ':')
		dst = appendEscaped(dst, header.Value)
		dst = append(dst, '\n')
	}
	if frame.Body != nil && !hasContentLength {
		dst = append(dst, "content-length:"...)
		dst = strconv.AppendInt(dst, int64(bodyLen), 10)
		dst = append(dst, '\n')
	}
	dst = append(dst, '\n')
	return len(dst) - start
}

func writeEscaped(out buffer.ByteBuf, src string, tmp []byte) error {
	if escapedLen(src) == len(src) {
		_, err := out.WriteBytes([]byte(src))
		return err
	}
	for i := 0; i < len(src); i++ {
		tmp = tmp[:0]
		switch src[i] {
		case '\n':
			tmp = append(tmp, '\\', 'n')
		case '\r':
			tmp = append(tmp, '\\', 'r')
		case ':':
			tmp = append(tmp, '\\', 'c')
		case '\\':
			tmp = append(tmp, '\\', '\\')
		default:
			tmp = append(tmp, src[i])
		}
		if _, err := out.WriteBytes(tmp); err != nil {
			return err
		}
	}
	return nil
}

func decimalLen(n int) int {
	if n == 0 {
		return 1
	}
	digits := 0
	for n > 0 {
		digits++
		n /= 10
	}
	return digits
}

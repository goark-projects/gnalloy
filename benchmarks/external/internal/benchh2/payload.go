package benchh2

// ResponseBody 返回固定响应体，确保所有框架返回同样字节数。
func ResponseBody(size int) []byte {
	body := make([]byte, size)
	for i := range body {
		body[i] = byte(i)
	}
	return body
}

func requestHeaderBlock(host string, tlsEnabled bool) []byte {
	if host == "" {
		host = "127.0.0.1"
	}
	out := make([]byte, 0, 16+len(host))
	out = append(out, 0x82)
	if tlsEnabled {
		out = append(out, 0x87)
	} else {
		out = append(out, 0x86)
	}
	out = appendLiteralIndexedName(out, 4, "/bench")
	out = appendLiteralIndexedName(out, 1, host)
	return out
}

func appendLiteralIndexedName(dst []byte, nameIndex byte, value string) []byte {
	dst = append(dst, nameIndex&0x0f)
	return appendHpackString(dst, value)
}

func appendHpackString(dst []byte, value string) []byte {
	n := len(value)
	if n < 127 {
		dst = append(dst, byte(n))
	} else {
		dst = append(dst, 127)
		n -= 127
		for n >= 128 {
			dst = append(dst, byte(n%128+128))
			n /= 128
		}
		dst = append(dst, byte(n))
	}
	return append(dst, value...)
}

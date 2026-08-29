package benchhttp

import (
	"strconv"
	"strings"
)

// ResponseBody 返回固定响应体，确保所有框架返回同样字节数。
func ResponseBody(size int) []byte {
	body := make([]byte, size)
	for i := range body {
		body[i] = byte(i)
	}
	return body
}

// ResponseBytes 返回 HTTP/1.1 keep-alive 固定响应帧。
func ResponseBytes(payload int) []byte {
	body := ResponseBody(payload)
	header := "HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: " +
		strconv.Itoa(len(body)) + "\r\nConnection: keep-alive\r\n\r\n"
	out := make([]byte, 0, len(header)+len(body))
	out = append(out, header...)
	out = append(out, body...)
	return out
}

// RequestBytes 返回固定 GET 请求，payload 表示响应体大小而非请求体大小。
func RequestBytes(host string) []byte {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	return []byte("GET /bench HTTP/1.1\r\nHost: " + host + "\r\nUser-Agent: gnalloy-bench\r\nAccept: */*\r\nConnection: keep-alive\r\n\r\n")
}

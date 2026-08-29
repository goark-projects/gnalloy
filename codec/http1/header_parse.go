package http1

import (
	"strconv"
	"strings"

	"goark.dev/gnalloy/codec"
)

// parseRequestHeader 解析完整 HTTP/1 请求头，不为行列表创建临时切片。
func parseRequestHeader(src string) (Request, error) {
	line, next, ok := nextHeaderLine(src, 0)
	if !ok {
		return Request{}, codec.ErrInvalidFrameLength
	}
	method, uri, version, ok := splitRequestLine(line)
	if !ok {
		return Request{}, codec.ErrInvalidFrameLength
	}
	headers, err := parseHeaderFields(src, next)
	if err != nil {
		return Request{}, err
	}
	return Request{Method: method, URI: uri, Version: version, Headers: headers}, nil
}

// parseResponseHeader 解析完整 HTTP/1 响应头，不为行列表创建临时切片。
func parseResponseHeader(src string) (Response, error) {
	line, next, ok := nextHeaderLine(src, 0)
	if !ok {
		return Response{}, codec.ErrInvalidFrameLength
	}
	version, statusText, reason, ok := splitResponseLine(line)
	if !ok {
		return Response{}, codec.ErrInvalidFrameLength
	}
	statusCode, err := strconv.Atoi(statusText)
	if err != nil {
		return Response{}, codec.ErrInvalidFrameLength
	}
	headers, err := parseHeaderFields(src, next)
	if err != nil {
		return Response{}, err
	}
	return Response{Version: version, StatusCode: statusCode, Reason: reason, Headers: headers}, nil
}

func parseTrailerHeaders(src string) (Headers, error) {
	return parseHeaderFields(src, 0)
}

func contentLength(headers Headers) int {
	for k, v := range headers {
		if !strings.EqualFold(k, "Content-Length") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return -1
		}
		return n
	}
	return 0
}

func splitRequestLine(line string) (string, string, string, bool) {
	first := strings.IndexByte(line, ' ')
	if first < 0 {
		return "", "", "", false
	}
	second := strings.IndexByte(line[first+1:], ' ')
	if second < 0 {
		return "", "", "", false
	}
	second += first + 1
	return line[:first], line[first+1 : second], line[second+1:], true
}

func splitResponseLine(line string) (string, string, string, bool) {
	first := strings.IndexByte(line, ' ')
	if first < 0 {
		return "", "", "", false
	}
	second := strings.IndexByte(line[first+1:], ' ')
	if second < 0 {
		return line[:first], line[first+1:], "", true
	}
	second += first + 1
	return line[:first], line[first+1 : second], line[second+1:], true
}

func parseHeaderFields(src string, start int) (Headers, error) {
	headers := make(Headers, 4)
	for start < len(src) {
		line, next, ok := nextHeaderLine(src, start)
		if !ok {
			return nil, codec.ErrInvalidFrameLength
		}
		start = next
		if line == "" {
			return headers, nil
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			return nil, codec.ErrInvalidFrameLength
		}
		headers[strings.TrimSpace(line[:colon])] = strings.TrimSpace(line[colon+1:])
	}
	return headers, nil
}

func nextHeaderLine(src string, start int) (string, int, bool) {
	if start > len(src) {
		return "", start, false
	}
	end := strings.Index(src[start:], "\r\n")
	if end < 0 {
		return "", start, false
	}
	end += start
	return src[start:end], end + 2, true
}

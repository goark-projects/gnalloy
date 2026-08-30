package http1

import (
	"strconv"
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

const contentLengthHeader = "Content-Length: "

func requestChunked(req Request) bool {
	return req.Headers.ContainsToken("Transfer-Encoding", "chunked")
}

func responseChunked(resp Response) bool {
	return resp.Headers.ContainsToken("Transfer-Encoding", "chunked")
}

func encodeRequestHead(ctx *channel.HandlerContext, req Request, chunked bool) (buffer.ByteBuf, error) {
	method := req.Method
	if method == "" {
		method = "GET"
	}
	uri := req.URI
	if uri == "" {
		uri = "/"
	}
	version := req.Version
	if version == "" {
		version = "HTTP/1.1"
	}
	contentLength := -1
	if req.Body != nil && !chunked && !hasHeader(req.Headers, "Content-Length") {
		contentLength = req.Body.ReadableBytes()
	}
	size := requestHeadSize(method, uri, version, req.Headers, contentLength)
	out, err := ctx.Channel().Allocator().Acquire(size)
	if err != nil {
		return nil, err
	}
	if err := appendRequestHead(out, method, uri, version, req.Headers, contentLength); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

func encodeResponseHead(ctx *channel.HandlerContext, resp Response, chunked bool) (buffer.ByteBuf, error) {
	version, reason, contentLength, size := responseHeadFields(resp, chunked)
	out, err := ctx.Channel().Allocator().Acquire(size)
	if err != nil {
		return nil, err
	}
	if err := appendResponseHead(out, version, resp.StatusCode, reason, resp.Headers, contentLength); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

func encodeResponse(ctx *channel.HandlerContext, resp Response, bodyBytes int) (buffer.ByteBuf, error) {
	version, reason, contentLength, size := responseHeadFields(resp, false)
	out, err := ctx.Channel().Allocator().Acquire(size + bodyBytes)
	if err != nil {
		return nil, err
	}
	if err := appendResponseHead(out, version, resp.StatusCode, reason, resp.Headers, contentLength); err != nil {
		out.Release()
		return nil, err
	}
	if err := buffer.WriteReadableBytes(out, resp.Body); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

func responseHeadFields(resp Response, chunked bool) (string, string, int, int) {
	version := resp.Version
	if version == "" {
		version = "HTTP/1.1"
	}
	reason := resp.Reason
	if reason == "" {
		reason = defaultReason(resp.StatusCode)
	}
	contentLength := -1
	if resp.Body != nil && !chunked && !hasHeader(resp.Headers, "Content-Length") {
		contentLength = resp.Body.ReadableBytes()
	}
	size := responseHeadSize(version, resp.StatusCode, reason, resp.Headers, contentLength)
	return version, reason, contentLength, size
}

func appendRequestHead(out buffer.ByteBuf, method string, uri string, version string, headers Headers, contentLength int) error {
	dst := out.WritableBytesView()
	if len(dst) == 0 {
		return buffer.ErrNoWritableBytes
	}
	dst = dst[:0]
	dst = append(dst, method...)
	dst = append(dst, ' ')
	dst = append(dst, uri...)
	dst = append(dst, ' ')
	dst = append(dst, version...)
	dst = appendCRLF(dst)
	dst = appendHeaders(dst, headers)
	dst = appendContentLength(dst, contentLength)
	dst = appendCRLF(dst)
	return out.AdvanceWriter(len(dst))
}

func appendResponseHead(out buffer.ByteBuf, version string, statusCode int, reason string, headers Headers, contentLength int) error {
	dst := out.WritableBytesView()
	if len(dst) == 0 {
		return buffer.ErrNoWritableBytes
	}
	dst = dst[:0]
	dst = append(dst, version...)
	dst = append(dst, ' ')
	dst = strconv.AppendInt(dst, int64(statusCode), 10)
	dst = append(dst, ' ')
	dst = append(dst, reason...)
	dst = appendCRLF(dst)
	dst = appendHeaders(dst, headers)
	dst = appendContentLength(dst, contentLength)
	dst = appendCRLF(dst)
	return out.AdvanceWriter(len(dst))
}

func requestHeadSize(method string, uri string, version string, headers Headers, contentLength int) int {
	return len(method) + 1 + len(uri) + 1 + len(version) + 2 +
		headersSize(headers) + contentLengthSize(contentLength) + 2
}

func responseHeadSize(version string, statusCode int, reason string, headers Headers, contentLength int) int {
	return len(version) + 1 + intTextLen(statusCode) + 1 + len(reason) + 2 +
		headersSize(headers) + contentLengthSize(contentLength) + 2
}

func headersSize(headers Headers) int {
	size := 0
	for k, v := range headers {
		size += len(k) + 2 + len(v) + 2
	}
	return size
}

func contentLengthSize(contentLength int) int {
	if contentLength < 0 {
		return 0
	}
	return len(contentLengthHeader) + intTextLen(contentLength) + 2
}

func appendHeaders(dst []byte, headers Headers) []byte {
	for k, v := range headers {
		dst = append(dst, k...)
		dst = append(dst, ':', ' ')
		dst = append(dst, v...)
		dst = appendCRLF(dst)
	}
	return dst
}

func appendContentLength(dst []byte, contentLength int) []byte {
	if contentLength < 0 {
		return dst
	}
	dst = append(dst, contentLengthHeader...)
	dst = strconv.AppendInt(dst, int64(contentLength), 10)
	return appendCRLF(dst)
}

func appendCRLF(dst []byte) []byte {
	return append(dst, '\r', '\n')
}

func hasHeader(headers Headers, name string) bool {
	for k := range headers {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}

func intTextLen(n int) int {
	if n == 0 {
		return 1
	}
	size := 0
	if n < 0 {
		size++
		n = -n
	}
	for n > 0 {
		size++
		n /= 10
	}
	return size
}

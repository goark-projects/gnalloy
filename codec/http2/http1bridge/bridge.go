package http1bridge

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/codec/http2"
)

const defaultScheme = "https"

var ErrInvalidHeadersBlock = errors.New("gnalloy/codec/http2/http1bridge: invalid headers block")

// RequestFromHeadersBlock 将 HTTP/2 伪头转换为 HTTP/1 请求对象语义。
func RequestFromHeadersBlock(block http2.HeadersBlock) (http1.Request, error) {
	req := http1.Request{Version: "HTTP/1.1", Headers: http1.Headers{}}
	for _, field := range block.Fields {
		switch strings.ToLower(field.Name) {
		case ":method":
			req.Method = field.Value
		case ":path":
			req.URI = field.Value
		case ":authority":
			setHeader(req.Headers, "Host", field.Value)
		case ":scheme":
			continue
		default:
			if strings.HasPrefix(field.Name, ":") {
				return http1.Request{}, ErrInvalidHeadersBlock
			}
			setHeader(req.Headers, field.Name, field.Value)
		}
	}
	if req.Method == "" || req.URI == "" {
		return http1.Request{}, ErrInvalidHeadersBlock
	}
	return req, nil
}

// ResponseFromHeadersBlock 将 HTTP/2 :status 伪头转换为 HTTP/1 响应对象语义。
func ResponseFromHeadersBlock(block http2.HeadersBlock) (http1.Response, error) {
	resp := http1.Response{Version: "HTTP/1.1", Headers: http1.Headers{}}
	for _, field := range block.Fields {
		switch strings.ToLower(field.Name) {
		case ":status":
			code, err := strconv.Atoi(field.Value)
			if err != nil || code < 100 || code > 999 {
				return http1.Response{}, ErrInvalidHeadersBlock
			}
			resp.StatusCode = code
		default:
			if strings.HasPrefix(field.Name, ":") {
				return http1.Response{}, ErrInvalidHeadersBlock
			}
			setHeader(resp.Headers, field.Name, field.Value)
		}
	}
	if resp.StatusCode == 0 {
		return http1.Response{}, ErrInvalidHeadersBlock
	}
	return resp, nil
}

// HeadersBlockFromRequest 将 HTTP/1 请求对象转换为 HTTP/2 header block，伪头始终排在最前。
func HeadersBlockFromRequest(streamID http2.StreamID, req http1.Request, endStream bool) http2.HeadersBlock {
	method := req.Method
	if method == "" {
		method = "GET"
	}
	path := req.URI
	if path == "" {
		path = "/"
	}
	fields := []http2.HeaderField{
		{Name: ":method", Value: method},
		{Name: ":scheme", Value: defaultScheme},
		{Name: ":authority", Value: req.Headers.Get("Host")},
		{Name: ":path", Value: path},
	}
	fields = appendRegularHeaders(fields, req.Headers)
	return http2.HeadersBlock{StreamID: streamID, Fields: fields, EndStream: endStream}
}

// HeadersBlockFromResponse 将 HTTP/1 响应对象转换为 HTTP/2 header block，:status 始终排在最前。
func HeadersBlockFromResponse(streamID http2.StreamID, resp http1.Response, endStream bool) http2.HeadersBlock {
	code := resp.StatusCode
	if code == 0 {
		code = 200
	}
	fields := []http2.HeaderField{{Name: ":status", Value: strconv.Itoa(code)}}
	fields = appendRegularHeaders(fields, resp.Headers)
	return http2.HeadersBlock{StreamID: streamID, Fields: fields, EndStream: endStream}
}

func appendRegularHeaders(fields []http2.HeaderField, headers http1.Headers) []http2.HeaderField {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Slice(names, func(i int, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	for _, name := range names {
		value := headers[name]
		lower := strings.ToLower(name)
		if !allowedHTTP2Header(lower, value) {
			continue
		}
		fields = append(fields, http2.HeaderField{Name: lower, Value: value})
	}
	return fields
}

func allowedHTTP2Header(name string, value string) bool {
	switch name {
	case "host", "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade":
		return false
	case "te":
		return strings.EqualFold(strings.TrimSpace(value), "trailers")
	default:
		return !strings.HasPrefix(name, ":")
	}
}

func setHeader(headers http1.Headers, name string, value string) {
	for existing, old := range headers {
		if strings.EqualFold(existing, name) {
			headers[existing] = old + ", " + value
			return
		}
	}
	headers[name] = value
}

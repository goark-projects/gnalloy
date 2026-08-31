package http1bridge

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/codec/http3"
)

const defaultScheme = "https"

var ErrInvalidHeadersBlock = errors.New("gnalloy/codec/http3/http1bridge: invalid headers block")

// PushPromise 表示 HTTP/3 PUSH_PROMISE 对应的 HTTP 请求对象语义。
type PushPromise struct {
	PushID  uint64
	Request http1.Request
}

// Release 释放 push promise 中可能携带的请求正文。
func (p PushPromise) Release() {
	p.Request.Release()
}

// RequestFromHeadersBlock 将 HTTP/3 请求伪头转换为 HTTP/1 请求对象语义。
func RequestFromHeadersBlock(block http3.HeadersBlock) (http1.Request, error) {
	req := http1.Request{Version: "HTTP/1.1", Headers: http1.Headers{}}
	seen := headerBlockState{pseudo: make(map[string]struct{}, 4)}
	for _, field := range block.Fields {
		name := strings.ToLower(field.Name)
		if err := seen.validate(field.Name, name); err != nil {
			return http1.Request{}, err
		}
		switch name {
		case ":method":
			req.Method = field.Value
		case ":scheme":
			continue
		case ":authority":
			setHeader(req.Headers, "Host", field.Value)
		case ":path":
			req.URI = field.Value
		default:
			if strings.HasPrefix(name, ":") {
				return http1.Request{}, ErrInvalidHeadersBlock
			}
			setHeader(req.Headers, name, field.Value)
		}
	}
	if req.Method == "" || req.URI == "" {
		return http1.Request{}, ErrInvalidHeadersBlock
	}
	return req, nil
}

// ResponseFromHeadersBlock 将 HTTP/3 :status 伪头转换为 HTTP/1 响应对象语义。
func ResponseFromHeadersBlock(block http3.HeadersBlock) (http1.Response, error) {
	resp := http1.Response{Version: "HTTP/1.1", Headers: http1.Headers{}}
	seen := headerBlockState{pseudo: make(map[string]struct{}, 1)}
	for _, field := range block.Fields {
		name := strings.ToLower(field.Name)
		if err := seen.validate(field.Name, name); err != nil {
			return http1.Response{}, err
		}
		switch name {
		case ":status":
			code, err := strconv.Atoi(field.Value)
			if err != nil || code < 100 || code > 999 {
				return http1.Response{}, ErrInvalidHeadersBlock
			}
			resp.StatusCode = code
		default:
			if strings.HasPrefix(name, ":") {
				return http1.Response{}, ErrInvalidHeadersBlock
			}
			setHeader(resp.Headers, name, field.Value)
		}
	}
	if resp.StatusCode == 0 {
		return http1.Response{}, ErrInvalidHeadersBlock
	}
	return resp, nil
}

// PushPromiseFromBlock 将 HTTP/3 PUSH_PROMISE 转换为带 push ID 的请求对象语义。
func PushPromiseFromBlock(block http3.PushPromiseBlock) (PushPromise, error) {
	req, err := RequestFromHeadersBlock(http3.HeadersBlock{Fields: block.Fields})
	if err != nil {
		return PushPromise{}, err
	}
	return PushPromise{PushID: block.PushID, Request: req}, nil
}

// HeadersBlockFromRequest 将 HTTP/1 请求对象转换为 HTTP/3 header block。
func HeadersBlockFromRequest(req http1.Request, scheme string) http3.HeadersBlock {
	if scheme == "" {
		scheme = defaultScheme
	}
	method := req.Method
	if method == "" {
		method = "GET"
	}
	path := req.URI
	if path == "" {
		path = "/"
	}
	fields := []http3.HeaderField{
		{Name: ":method", Value: method},
		{Name: ":scheme", Value: scheme},
	}
	if authority := req.Headers.Get("Host"); authority != "" {
		fields = append(fields, http3.HeaderField{Name: ":authority", Value: authority})
	}
	fields = append(fields, http3.HeaderField{Name: ":path", Value: path})
	fields = appendRegularHeaders(fields, req.Headers)
	return http3.HeadersBlock{Fields: fields}
}

// HeadersBlockFromResponse 将 HTTP/1 响应对象转换为 HTTP/3 header block。
func HeadersBlockFromResponse(resp http1.Response) http3.HeadersBlock {
	code := resp.StatusCode
	if code == 0 {
		code = 200
	}
	fields := []http3.HeaderField{{Name: ":status", Value: strconv.Itoa(code)}}
	fields = appendRegularHeaders(fields, resp.Headers)
	return http3.HeadersBlock{Fields: fields}
}

// HeadersBlockFromTrailers 将 HTTP/1 trailer 转换为 HTTP/3 trailing headers。
func HeadersBlockFromTrailers(trailers http1.Headers) http3.HeadersBlock {
	return http3.HeadersBlock{Fields: appendRegularHeaders(nil, trailers)}
}

// TrailersFromHeadersBlock 将 HTTP/3 trailing headers 转换为 HTTP/1 trailer。
func TrailersFromHeadersBlock(block http3.HeadersBlock) (http1.Headers, error) {
	trailers := http1.Headers{}
	for _, field := range block.Fields {
		name := strings.ToLower(field.Name)
		if field.Name != name || strings.HasPrefix(name, ":") || !allowedHTTP3Header(name, field.Value) {
			return nil, ErrInvalidHeadersBlock
		}
		setHeader(trailers, name, field.Value)
	}
	return trailers, nil
}

type headerBlockState struct {
	pseudo      map[string]struct{}
	seenRegular bool
}

func (s *headerBlockState) validate(raw string, lower string) error {
	if raw != lower {
		return ErrInvalidHeadersBlock
	}
	if strings.HasPrefix(lower, ":") {
		if s.seenRegular {
			return ErrInvalidHeadersBlock
		}
		if _, exists := s.pseudo[lower]; exists {
			return ErrInvalidHeadersBlock
		}
		s.pseudo[lower] = struct{}{}
		return nil
	}
	s.seenRegular = true
	return nil
}

func appendRegularHeaders(fields []http3.HeaderField, headers http1.Headers) []http3.HeaderField {
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
		if !allowedHTTP3Header(lower, value) {
			continue
		}
		fields = append(fields, http3.HeaderField{Name: lower, Value: value})
	}
	return fields
}

func allowedHTTP3Header(name string, value string) bool {
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

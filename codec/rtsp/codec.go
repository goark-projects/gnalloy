package rtsp

import (
	"strconv"
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

type RequestDecoder struct {
	*codec.ByteToMessageDecoder
	maxHeaderBytes int
	maxBodyBytes   int
}

func NewRequestDecoder(maxHeaderBytes int, maxBodyBytes int) (*RequestDecoder, error) {
	if maxHeaderBytes <= 0 || maxBodyBytes < 0 {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &RequestDecoder{maxHeaderBytes: maxHeaderBytes, maxBodyBytes: maxBodyBytes}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

func (d *RequestDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	headerEnd, ok := findHeaderEnd(in)
	if !ok {
		if in.ReadableBytes() > d.maxHeaderBytes {
			return nil, codec.ErrFrameTooLong
		}
		return nil, nil
	}
	reader := in.ReaderIndex()
	headerBytes := headerEnd - reader + 4
	if headerBytes > d.maxHeaderBytes {
		return nil, codec.ErrFrameTooLong
	}
	header, err := stringSlice(in, reader, headerBytes)
	if err != nil {
		return nil, err
	}
	req, err := parseRequestHeader(header)
	if err != nil {
		return nil, err
	}
	bodyLength := contentLength(req.Headers)
	if bodyLength < 0 || bodyLength > d.maxBodyBytes {
		return nil, codec.ErrFrameTooLong
	}
	total := headerBytes + bodyLength
	if in.ReadableBytes() < total {
		return nil, nil
	}
	if bodyLength > 0 {
		req.Body, err = in.Slice(reader+headerBytes, bodyLength)
		if err != nil {
			return nil, err
		}
	}
	if err := in.SkipBytes(total); err != nil {
		req.Release()
		return nil, err
	}
	return req, nil
}

type ResponseDecoder struct {
	*codec.ByteToMessageDecoder
	maxHeaderBytes int
	maxBodyBytes   int
}

func NewResponseDecoder(maxHeaderBytes int, maxBodyBytes int) (*ResponseDecoder, error) {
	if maxHeaderBytes <= 0 || maxBodyBytes < 0 {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &ResponseDecoder{maxHeaderBytes: maxHeaderBytes, maxBodyBytes: maxBodyBytes}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

func (d *ResponseDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	headerEnd, ok := findHeaderEnd(in)
	if !ok {
		if in.ReadableBytes() > d.maxHeaderBytes {
			return nil, codec.ErrFrameTooLong
		}
		return nil, nil
	}
	reader := in.ReaderIndex()
	headerBytes := headerEnd - reader + 4
	if headerBytes > d.maxHeaderBytes {
		return nil, codec.ErrFrameTooLong
	}
	header, err := stringSlice(in, reader, headerBytes)
	if err != nil {
		return nil, err
	}
	resp, err := parseResponseHeader(header)
	if err != nil {
		return nil, err
	}
	bodyLength := contentLength(resp.Headers)
	if bodyLength < 0 || bodyLength > d.maxBodyBytes {
		return nil, codec.ErrFrameTooLong
	}
	total := headerBytes + bodyLength
	if in.ReadableBytes() < total {
		return nil, nil
	}
	if bodyLength > 0 {
		resp.Body, err = in.Slice(reader+headerBytes, bodyLength)
		if err != nil {
			return nil, err
		}
	}
	if err := in.SkipBytes(total); err != nil {
		resp.Release()
		return nil, err
	}
	return resp, nil
}

type RequestEncoder struct{}

func NewRequestEncoder() *RequestEncoder {
	return &RequestEncoder{}
}

func (e *RequestEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	req, ok := msg.(Request)
	if !ok {
		return ctx.Write(msg)
	}
	method := req.Method
	if method == "" {
		method = MethodOptions
	}
	uri := req.URI
	if uri == "" {
		uri = "*"
	}
	version := req.Version
	if version == "" {
		version = Version10
	}
	return writeStartAndHeaders(ctx, string(method)+" "+uri+" "+version, req.Headers, req.Body)
}

type ResponseEncoder struct{}

func NewResponseEncoder() *ResponseEncoder {
	return &ResponseEncoder{}
}

func (e *ResponseEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	resp, ok := msg.(Response)
	if !ok {
		return ctx.Write(msg)
	}
	version := resp.Version
	if version == "" {
		version = Version10
	}
	reason := resp.Reason
	if reason == "" {
		reason = defaultReason(resp.StatusCode)
	}
	return writeStartAndHeaders(ctx, version+" "+strconv.Itoa(resp.StatusCode)+" "+reason, resp.Headers, resp.Body)
}

func writeStartAndHeaders(ctx *channel.HandlerContext, start string, headers Headers, body buffer.ByteBuf) error {
	var b strings.Builder
	b.WriteString(start)
	b.WriteString("\r\n")
	hasContentLength := false
	for k, v := range headers {
		if strings.EqualFold(k, "Content-Length") {
			hasContentLength = true
		}
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteString("\r\n")
	}
	if body != nil && !hasContentLength {
		b.WriteString("Content-Length: ")
		b.WriteString(strconv.Itoa(body.ReadableBytes()))
		b.WriteString("\r\n")
	}
	b.WriteString("\r\n")
	headerText := b.String()
	out, err := ctx.Channel().Allocator().Acquire(len(headerText))
	if err != nil {
		if body != nil {
			body.Release()
		}
		return err
	}
	if _, err := out.WriteBytes([]byte(headerText)); err != nil {
		out.Release()
		if body != nil {
			body.Release()
		}
		return err
	}
	if err := ctx.Write(out); err != nil {
		out.Release()
		if body != nil {
			body.Release()
		}
		return err
	}
	if body != nil {
		return codec.WriteOutboundBuffer(ctx, body)
	}
	return nil
}

func findHeaderEnd(in *buffer.CompositeByteBuf) (int, bool) {
	for i := in.ReaderIndex(); i+3 < in.WriterIndex(); i++ {
		a, _ := in.GetByte(i)
		b, _ := in.GetByte(i + 1)
		c, _ := in.GetByte(i + 2)
		d, _ := in.GetByte(i + 3)
		if a == '\r' && b == '\n' && c == '\r' && d == '\n' {
			return i, true
		}
	}
	return 0, false
}

func stringSlice(in *buffer.CompositeByteBuf, index int, length int) (string, error) {
	part, err := in.Slice(index, length)
	if err != nil {
		return "", err
	}
	defer part.Release()
	return string(part.Bytes()), nil
}

func parseRequestHeader(src string) (Request, error) {
	lines := strings.Split(src, "\r\n")
	if len(lines) == 0 {
		return Request{}, ErrInvalidMessage
	}
	parts := strings.SplitN(lines[0], " ", 3)
	if len(parts) != 3 || parts[2] != Version10 {
		return Request{}, ErrInvalidMessage
	}
	req := Request{Method: Method(parts[0]), URI: parts[1], Version: parts[2], Headers: make(Headers, len(lines))}
	for _, line := range lines[1:] {
		if line == "" {
			break
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return Request{}, ErrInvalidMessage
		}
		req.Headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return req, nil
}

func parseResponseHeader(src string) (Response, error) {
	lines := strings.Split(src, "\r\n")
	if len(lines) == 0 {
		return Response{}, ErrInvalidMessage
	}
	parts := strings.SplitN(lines[0], " ", 3)
	if len(parts) < 2 || parts[0] != Version10 {
		return Response{}, ErrInvalidMessage
	}
	status, err := strconv.Atoi(parts[1])
	if err != nil {
		return Response{}, ErrInvalidMessage
	}
	reason := ""
	if len(parts) == 3 {
		reason = parts[2]
	}
	resp := Response{Version: parts[0], StatusCode: status, Reason: reason, Headers: make(Headers, len(lines))}
	for _, line := range lines[1:] {
		if line == "" {
			break
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return Response{}, ErrInvalidMessage
		}
		resp.Headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return resp, nil
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

func defaultReason(code int) string {
	switch code {
	case 200:
		return "OK"
	case 400:
		return "Bad Request"
	case 404:
		return "Not Found"
	case 454:
		return "Session Not Found"
	case 500:
		return "Internal Server Error"
	default:
		return "Status"
	}
}

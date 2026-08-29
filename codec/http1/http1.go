package http1

import (
	"strconv"
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

var (
	crlfBytes      = []byte{'\r', '\n'}
	headerEndBytes = []byte{'\r', '\n', '\r', '\n'}
)

type Headers map[string]string

func (h Headers) Get(name string) string {
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func (h Headers) Set(name string, value string) {
	h[name] = value
}

func (h Headers) Del(name string) {
	for k := range h {
		if strings.EqualFold(k, name) {
			delete(h, k)
			return
		}
	}
}

func (h Headers) ContainsToken(name string, token string) bool {
	value := h.Get(name)
	for part := range strings.SplitSeq(value, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

type Request struct {
	Method  string
	URI     string
	Version string
	Headers Headers
	Body    buffer.ByteBuf
}

func (r Request) KeepAlive() bool {
	if r.Headers.ContainsToken("Connection", "close") {
		return false
	}
	if r.Version == "HTTP/1.0" {
		return r.Headers.ContainsToken("Connection", "keep-alive")
	}
	return true
}

func (r Request) ExpectsContinue() bool {
	return r.Headers.ContainsToken("Expect", "100-continue")
}

func (r Request) Release() {
	if r.Body != nil {
		r.Body.Release()
	}
}

type Response struct {
	Version    string
	StatusCode int
	Reason     string
	Headers    Headers
	Body       buffer.ByteBuf
}

func (r Response) KeepAlive() bool {
	if r.Headers.ContainsToken("Connection", "close") {
		return false
	}
	if r.Version == "HTTP/1.0" {
		return r.Headers.ContainsToken("Connection", "keep-alive")
	}
	return true
}

func (r Response) Release() {
	if r.Body != nil {
		r.Body.Release()
	}
}

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

func (d *RequestDecoder) Decode(ctx *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
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
	if req.Headers.ContainsToken("Transfer-Encoding", "chunked") {
		body, total, ok, err := d.decodeChunkedBody(ctx, in, reader+headerBytes)
		if err != nil || !ok {
			return nil, err
		}
		req.Body = body
		req.Headers.Del("Transfer-Encoding")
		req.Headers.Set("Content-Length", strconv.Itoa(body.ReadableBytes()))
		if err := in.SkipBytes(headerBytes + total); err != nil {
			req.Release()
			return nil, err
		}
		return req, nil
	}
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
		if req.Body != nil {
			req.Body.Release()
		}
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

func (d *ResponseDecoder) Decode(ctx *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
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
	if resp.Headers.ContainsToken("Transfer-Encoding", "chunked") {
		body, total, ok, err := d.decodeChunkedBody(ctx, in, reader+headerBytes)
		if err != nil || !ok {
			return nil, err
		}
		resp.Body = body
		resp.Headers.Del("Transfer-Encoding")
		resp.Headers.Set("Content-Length", strconv.Itoa(body.ReadableBytes()))
		if err := in.SkipBytes(headerBytes + total); err != nil {
			resp.Release()
			return nil, err
		}
		return resp, nil
	}
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
		if resp.Body != nil {
			resp.Body.Release()
		}
		return nil, err
	}
	return resp, nil
}

func (d *ResponseDecoder) decodeChunkedBody(ctx *channel.HandlerContext, in *buffer.CompositeByteBuf, index int) (buffer.ByteBuf, int, bool, error) {
	return decodeChunkedBody(ctx, in, index, d.maxBodyBytes)
}

func (d *RequestDecoder) decodeChunkedBody(ctx *channel.HandlerContext, in *buffer.CompositeByteBuf, index int) (buffer.ByteBuf, int, bool, error) {
	return decodeChunkedBody(ctx, in, index, d.maxBodyBytes)
}

func decodeChunkedBody(ctx *channel.HandlerContext, in *buffer.CompositeByteBuf, index int, maxBodyBytes int) (buffer.ByteBuf, int, bool, error) {
	total := 0
	aggregate, err := ctx.Channel().Allocator().Acquire(1)
	if err != nil {
		return nil, 0, false, err
	}
	aggregate.Clear()
	for {
		lineEnd, ok := findCRLF(in, index+total)
		if !ok {
			aggregate.Release()
			return nil, 0, false, nil
		}
		line, err := stringSlice(in, index+total, lineEnd-(index+total))
		if err != nil {
			aggregate.Release()
			return nil, 0, false, err
		}
		sizeText, _, _ := strings.Cut(line, ";")
		chunkSize64, err := strconv.ParseInt(strings.TrimSpace(sizeText), 16, 64)
		if err != nil || chunkSize64 < 0 {
			aggregate.Release()
			return nil, 0, false, codec.ErrInvalidFrameLength
		}
		chunkSize := int(chunkSize64)
		total += lineEnd - (index + total) + 2
		if chunkSize == 0 {
			if in.WriterIndex() < index+total+2 {
				aggregate.Release()
				return nil, 0, false, nil
			}
			total += 2
			return aggregate, total, true, nil
		}
		if aggregate.ReadableBytes()+chunkSize > maxBodyBytes {
			aggregate.Release()
			return nil, 0, false, codec.ErrFrameTooLong
		}
		if in.WriterIndex() < index+total+chunkSize+2 {
			aggregate.Release()
			return nil, 0, false, nil
		}
		part, err := in.Slice(index+total, chunkSize)
		if err != nil {
			aggregate.Release()
			return nil, 0, false, err
		}
		if aggregate.WritableBytes() < chunkSize {
			next, err := ctx.Channel().Allocator().Acquire(aggregate.ReadableBytes() + chunkSize)
			if err != nil {
				part.Release()
				aggregate.Release()
				return nil, 0, false, err
			}
			if err := buffer.WriteReadableBytes(next, aggregate); err != nil {
				next.Release()
				part.Release()
				aggregate.Release()
				return nil, 0, false, err
			}
			aggregate.Release()
			aggregate = next
		}
		if err := buffer.WriteReadableBytes(aggregate, part); err != nil {
			part.Release()
			aggregate.Release()
			return nil, 0, false, err
		}
		part.Release()
		total += chunkSize
		cr, _ := in.GetByte(index + total)
		lf, _ := in.GetByte(index + total + 1)
		if cr != '\r' || lf != '\n' {
			aggregate.Release()
			return nil, 0, false, codec.ErrInvalidFrameLength
		}
		total += 2
	}
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
	var builder strings.Builder
	builder.WriteString(method)
	builder.WriteByte(' ')
	builder.WriteString(uri)
	builder.WriteByte(' ')
	builder.WriteString(version)
	builder.WriteString("\r\n")
	hasContentLength := false
	chunked := req.Headers.ContainsToken("Transfer-Encoding", "chunked")
	for k, v := range req.Headers {
		if strings.EqualFold(k, "Content-Length") {
			hasContentLength = true
		}
		builder.WriteString(k)
		builder.WriteString(": ")
		builder.WriteString(v)
		builder.WriteString("\r\n")
	}
	if req.Body != nil && !hasContentLength && !chunked {
		builder.WriteString("Content-Length: ")
		builder.WriteString(strconv.Itoa(req.Body.ReadableBytes()))
		builder.WriteString("\r\n")
	}
	builder.WriteString("\r\n")
	header := builder.String()
	out, err := ctx.Channel().Allocator().Acquire(len(header))
	if err != nil {
		if req.Body != nil {
			req.Body.Release()
		}
		return err
	}
	if _, err := out.WriteBytes([]byte(header)); err != nil {
		out.Release()
		if req.Body != nil {
			req.Body.Release()
		}
		return err
	}
	if err := ctx.Write(out); err != nil {
		out.Release()
		if req.Body != nil {
			req.Body.Release()
		}
		return err
	}
	if req.Body != nil {
		if chunked {
			if err := writeChunkedData(ctx, req.Body); err != nil {
				return err
			}
			return writeLastChunk(ctx, nil)
		}
		return codec.WriteOutboundBuffer(ctx, req.Body)
	}
	return nil
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
		version = "HTTP/1.1"
	}
	reason := resp.Reason
	if reason == "" {
		reason = defaultReason(resp.StatusCode)
	}
	var builder strings.Builder
	builder.WriteString(version)
	builder.WriteByte(' ')
	builder.WriteString(strconv.Itoa(resp.StatusCode))
	builder.WriteByte(' ')
	builder.WriteString(reason)
	builder.WriteString("\r\n")
	hasContentLength := false
	chunked := resp.Headers.ContainsToken("Transfer-Encoding", "chunked")
	for k, v := range resp.Headers {
		if strings.EqualFold(k, "Content-Length") {
			hasContentLength = true
		}
		builder.WriteString(k)
		builder.WriteString(": ")
		builder.WriteString(v)
		builder.WriteString("\r\n")
	}
	if resp.Body != nil && !hasContentLength && !chunked {
		builder.WriteString("Content-Length: ")
		builder.WriteString(strconv.Itoa(resp.Body.ReadableBytes()))
		builder.WriteString("\r\n")
	}
	builder.WriteString("\r\n")
	header := builder.String()
	out, err := ctx.Channel().Allocator().Acquire(len(header))
	if err != nil {
		if resp.Body != nil {
			resp.Body.Release()
		}
		return err
	}
	if _, err := out.WriteBytes([]byte(header)); err != nil {
		out.Release()
		if resp.Body != nil {
			resp.Body.Release()
		}
		return err
	}
	if err := ctx.Write(out); err != nil {
		out.Release()
		if resp.Body != nil {
			resp.Body.Release()
		}
		return err
	}
	if resp.Body != nil {
		if chunked {
			if err := writeChunkedData(ctx, resp.Body); err != nil {
				return err
			}
			return writeLastChunk(ctx, nil)
		}
		return codec.WriteOutboundBuffer(ctx, resp.Body)
	}
	return nil
}

type ContinueHandler struct{}

func NewContinueHandler() *ContinueHandler {
	return &ContinueHandler{}
}

func (h *ContinueHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	req, ok := msg.(Request)
	if ok && req.ExpectsContinue() {
		if err := ctx.Channel().WriteAndFlush(Response{StatusCode: 100}); err != nil {
			req.Release()
			ctx.FireExceptionCaught(err)
			return
		}
	}
	ctx.FireChannelRead(msg)
}

type Chunk struct {
	Data     buffer.ByteBuf
	Last     bool
	Trailers Headers
}

func (c Chunk) Release() {
	if c.Data != nil {
		c.Data.Release()
	}
}

type ChunkedBodyEncoder struct{}

func NewChunkedBodyEncoder() *ChunkedBodyEncoder {
	return &ChunkedBodyEncoder{}
}

func (e *ChunkedBodyEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	chunk, ok := msg.(Chunk)
	if !ok {
		return ctx.Write(msg)
	}
	if chunk.Data != nil {
		if err := writeChunkedData(ctx, chunk.Data); err != nil {
			return err
		}
		chunk.Data = nil
	}
	if chunk.Last {
		return writeLastChunk(ctx, chunk.Trailers)
	}
	return nil
}

func writeChunkedData(ctx *channel.HandlerContext, body buffer.ByteBuf) error {
	prefix := strconv.FormatInt(int64(body.ReadableBytes()), 16) + "\r\n"
	head, err := ctx.Channel().Allocator().Acquire(len(prefix))
	if err != nil {
		body.Release()
		return err
	}
	if _, err := head.WriteBytes([]byte(prefix)); err != nil {
		head.Release()
		body.Release()
		return err
	}
	tail, err := ctx.Channel().Allocator().Acquire(2)
	if err != nil {
		head.Release()
		body.Release()
		return err
	}
	if _, err := tail.WriteBytes([]byte("\r\n")); err != nil {
		head.Release()
		tail.Release()
		body.Release()
		return err
	}
	if err := ctx.Write(head); err != nil {
		head.Release()
		tail.Release()
		body.Release()
		return err
	}
	if err := ctx.Write(body); err != nil {
		body.Release()
		tail.Release()
		return err
	}
	return codec.WriteOutboundBuffer(ctx, tail)
}

func writeLastChunk(ctx *channel.HandlerContext, trailers Headers) error {
	var builder strings.Builder
	builder.WriteString("0\r\n")
	for k, v := range trailers {
		builder.WriteString(k)
		builder.WriteString(": ")
		builder.WriteString(v)
		builder.WriteString("\r\n")
	}
	builder.WriteString("\r\n")
	data := builder.String()
	out, err := ctx.Channel().Allocator().Acquire(len(data))
	if err != nil {
		return err
	}
	if _, err := out.WriteBytes([]byte(data)); err != nil {
		out.Release()
		return err
	}
	return codec.WriteOutboundBuffer(ctx, out)
}

func findCRLF(in *buffer.CompositeByteBuf, start int) (int, bool) {
	return in.Index(start, crlfBytes)
}

func findHeaderEnd(in *buffer.CompositeByteBuf) (int, bool) {
	return findHeaderEndFrom(in, in.ReaderIndex())
}

func stringSlice(in *buffer.CompositeByteBuf, index int, length int) (string, error) {
	return buffer.ReadableStringAt(in, index, length)
}

func defaultReason(code int) string {
	switch code {
	case 100:
		return "Continue"
	case 101:
		return "Switching Protocols"
	case 200:
		return "OK"
	case 400:
		return "Bad Request"
	case 404:
		return "Not Found"
	case 500:
		return "Internal Server Error"
	default:
		return "Status"
	}
}

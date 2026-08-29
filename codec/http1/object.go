package http1

import (
	"strconv"
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

// Object 是 HTTP/1.x 对象流的公共生命周期约束。
type Object interface {
	Release()
}

// HTTPContent 表示一个普通 HTTP 正文分片，Data 的所有权随消息向后传递。
type HTTPContent struct {
	Data buffer.ByteBuf
}

func (c HTTPContent) Release() {
	if c.Data != nil {
		c.Data.Release()
	}
}

// LastHTTPContent 表示当前 HTTP 消息的最后一个正文分片，可携带 chunked trailer。
type LastHTTPContent struct {
	Data     buffer.ByteBuf
	Trailers Headers
}

func (c LastHTTPContent) Release() {
	if c.Data != nil {
		c.Data.Release()
	}
}

type httpObjectDecodeState struct {
	contentRemaining int
	chunked          bool
	chunkRead        int
	readingContent   bool
}

func (s *httpObjectDecodeState) reset() {
	s.contentRemaining = 0
	s.chunked = false
	s.chunkRead = 0
	s.readingContent = false
}

// RequestObjectDecoder 将 HTTP 请求解码为 Request + HTTPContent/LastHTTPContent 对象流。
type RequestObjectDecoder struct {
	*codec.ByteToMessageDecoder
	maxHeaderBytes  int
	maxContentBytes int
	state           httpObjectDecodeState
}

func NewRequestObjectDecoder(maxHeaderBytes int, maxContentBytes int) (*RequestObjectDecoder, error) {
	if maxHeaderBytes <= 0 || maxContentBytes < 0 {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &RequestObjectDecoder{maxHeaderBytes: maxHeaderBytes, maxContentBytes: maxContentBytes}
	d.ByteToMessageDecoder = codec.NewByteToMessageListDecoder(d)
	return d, nil
}

func (d *RequestObjectDecoder) DecodeBytes(_ *channel.HandlerContext, in *buffer.CompositeByteBuf, out *codec.MessageList) error {
	for in.ReadableBytes() > 0 {
		if !d.state.readingContent {
			ok, err := d.decodeHead(in, out)
			if err != nil || !ok {
				return err
			}
			continue
		}
		ok, err := decodeObjectContent(&d.state, d.maxContentBytes, in, out)
		if err != nil || !ok {
			return err
		}
	}
	return nil
}

func (d *RequestObjectDecoder) ChannelInactive(ctx *channel.HandlerContext) {
	d.state.reset()
	d.ByteToMessageDecoder.ChannelInactive(ctx)
}

func (d *RequestObjectDecoder) decodeHead(in *buffer.CompositeByteBuf, out *codec.MessageList) (bool, error) {
	headerEnd, ok := findHeaderEnd(in)
	if !ok {
		if in.ReadableBytes() > d.maxHeaderBytes {
			return false, codec.ErrFrameTooLong
		}
		return false, nil
	}
	reader := in.ReaderIndex()
	headerBytes := headerEnd - reader + 4
	if headerBytes > d.maxHeaderBytes {
		return false, codec.ErrFrameTooLong
	}
	header, err := stringSlice(in, reader, headerBytes)
	if err != nil {
		return false, err
	}
	req, err := parseRequestHeader(header)
	if err != nil {
		return false, err
	}
	if err := in.SkipBytes(headerBytes); err != nil {
		return false, err
	}
	out.Add(req)
	return true, startObjectContent(&d.state, req.Headers, d.maxContentBytes, out)
}

// ResponseObjectDecoder 将 HTTP 响应解码为 Response + HTTPContent/LastHTTPContent 对象流。
type ResponseObjectDecoder struct {
	*codec.ByteToMessageDecoder
	maxHeaderBytes  int
	maxContentBytes int
	state           httpObjectDecodeState
}

func NewResponseObjectDecoder(maxHeaderBytes int, maxContentBytes int) (*ResponseObjectDecoder, error) {
	if maxHeaderBytes <= 0 || maxContentBytes < 0 {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &ResponseObjectDecoder{maxHeaderBytes: maxHeaderBytes, maxContentBytes: maxContentBytes}
	d.ByteToMessageDecoder = codec.NewByteToMessageListDecoder(d)
	return d, nil
}

func (d *ResponseObjectDecoder) DecodeBytes(_ *channel.HandlerContext, in *buffer.CompositeByteBuf, out *codec.MessageList) error {
	for in.ReadableBytes() > 0 {
		if !d.state.readingContent {
			ok, err := d.decodeHead(in, out)
			if err != nil || !ok {
				return err
			}
			continue
		}
		ok, err := decodeObjectContent(&d.state, d.maxContentBytes, in, out)
		if err != nil || !ok {
			return err
		}
	}
	return nil
}

func (d *ResponseObjectDecoder) ChannelInactive(ctx *channel.HandlerContext) {
	d.state.reset()
	d.ByteToMessageDecoder.ChannelInactive(ctx)
}

func (d *ResponseObjectDecoder) decodeHead(in *buffer.CompositeByteBuf, out *codec.MessageList) (bool, error) {
	headerEnd, ok := findHeaderEnd(in)
	if !ok {
		if in.ReadableBytes() > d.maxHeaderBytes {
			return false, codec.ErrFrameTooLong
		}
		return false, nil
	}
	reader := in.ReaderIndex()
	headerBytes := headerEnd - reader + 4
	if headerBytes > d.maxHeaderBytes {
		return false, codec.ErrFrameTooLong
	}
	header, err := stringSlice(in, reader, headerBytes)
	if err != nil {
		return false, err
	}
	resp, err := parseResponseHeader(header)
	if err != nil {
		return false, err
	}
	if err := in.SkipBytes(headerBytes); err != nil {
		return false, err
	}
	out.Add(resp)
	return true, startObjectContent(&d.state, resp.Headers, d.maxContentBytes, out)
}

func startObjectContent(state *httpObjectDecodeState, headers Headers, maxContentBytes int, out *codec.MessageList) error {
	if headers.ContainsToken("Transfer-Encoding", "chunked") {
		state.chunked = true
		state.readingContent = true
		state.chunkRead = 0
		return nil
	}
	bodyLength := contentLength(headers)
	if bodyLength < 0 || bodyLength > maxContentBytes {
		return codec.ErrFrameTooLong
	}
	if bodyLength == 0 {
		out.Add(LastHTTPContent{})
		return nil
	}
	state.contentRemaining = bodyLength
	state.readingContent = true
	return nil
}

func decodeObjectContent(state *httpObjectDecodeState, maxContentBytes int, in *buffer.CompositeByteBuf, out *codec.MessageList) (bool, error) {
	if state.chunked {
		return decodeObjectChunk(state, maxContentBytes, in, out)
	}
	return decodeFixedObjectContent(state, in, out)
}

func decodeFixedObjectContent(state *httpObjectDecodeState, in *buffer.CompositeByteBuf, out *codec.MessageList) (bool, error) {
	if in.ReadableBytes() == 0 {
		return false, nil
	}
	n := state.contentRemaining
	if n > in.ReadableBytes() {
		n = in.ReadableBytes()
	}
	part, err := in.Slice(in.ReaderIndex(), n)
	if err != nil {
		return false, err
	}
	if err := in.SkipBytes(n); err != nil {
		part.Release()
		return false, err
	}
	state.contentRemaining -= n
	if state.contentRemaining == 0 {
		state.reset()
		out.Add(LastHTTPContent{Data: part})
		return true, nil
	}
	out.Add(HTTPContent{Data: part})
	return true, nil
}

func decodeObjectChunk(state *httpObjectDecodeState, maxContentBytes int, in *buffer.CompositeByteBuf, out *codec.MessageList) (bool, error) {
	reader := in.ReaderIndex()
	lineEnd, ok := findCRLF(in, reader)
	if !ok {
		return false, nil
	}
	line, err := stringSlice(in, reader, lineEnd-reader)
	if err != nil {
		return false, err
	}
	sizeText, _, _ := strings.Cut(line, ";")
	chunkSize64, err := strconv.ParseInt(strings.TrimSpace(sizeText), 16, 64)
	if err != nil || chunkSize64 < 0 {
		return false, codec.ErrInvalidFrameLength
	}
	chunkSize := int(chunkSize64)
	lineBytes := lineEnd - reader + 2
	if chunkSize == 0 {
		return decodeLastObjectChunk(state, in, lineBytes, out)
	}
	if state.chunkRead+chunkSize > maxContentBytes {
		return false, codec.ErrFrameTooLong
	}
	total := lineBytes + chunkSize + 2
	if in.ReadableBytes() < total {
		return false, nil
	}
	cr, _ := in.GetByte(reader + lineBytes + chunkSize)
	lf, _ := in.GetByte(reader + lineBytes + chunkSize + 1)
	if cr != '\r' || lf != '\n' {
		return false, codec.ErrInvalidFrameLength
	}
	part, err := in.Slice(reader+lineBytes, chunkSize)
	if err != nil {
		return false, err
	}
	if err := in.SkipBytes(total); err != nil {
		part.Release()
		return false, err
	}
	state.chunkRead += chunkSize
	out.Add(HTTPContent{Data: part})
	return true, nil
}

func decodeLastObjectChunk(state *httpObjectDecodeState, in *buffer.CompositeByteBuf, lineBytes int, out *codec.MessageList) (bool, error) {
	reader := in.ReaderIndex()
	trailerStart := reader + lineBytes
	if in.WriterIndex()-trailerStart < 2 {
		return false, nil
	}
	first, _ := in.GetByte(trailerStart)
	second, _ := in.GetByte(trailerStart + 1)
	trailerBytes := 2
	trailers := Headers{}
	if first != '\r' || second != '\n' {
		headerEnd, ok := findHeaderEndFrom(in, trailerStart)
		if !ok {
			return false, nil
		}
		trailerBytes = headerEnd - trailerStart + 4
		src, err := stringSlice(in, trailerStart, trailerBytes)
		if err != nil {
			return false, err
		}
		trailers, err = parseTrailerHeaders(src)
		if err != nil {
			return false, err
		}
	}
	if err := in.SkipBytes(lineBytes + trailerBytes); err != nil {
		return false, err
	}
	state.reset()
	out.Add(LastHTTPContent{Trailers: trailers})
	return true, nil
}

type httpAggregateKind uint8

const (
	httpAggregateNone httpAggregateKind = iota
	httpAggregateRequest
	httpAggregateResponse
)

// HTTPObjectAggregator 将对象流重新聚合成完整 Request 或 Response。
type HTTPObjectAggregator struct {
	maxContentBytes int
	kind            httpAggregateKind
	request         Request
	response        Response
	body            buffer.ByteBuf
}

func NewHTTPObjectAggregator(maxContentBytes int) *HTTPObjectAggregator {
	return &HTTPObjectAggregator{maxContentBytes: maxContentBytes}
}

func (a *HTTPObjectAggregator) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch v := msg.(type) {
	case Request:
		a.readRequest(ctx, v)
	case Response:
		a.readResponse(ctx, v)
	case HTTPContent:
		a.readContent(ctx, v.Data, false, nil)
	case LastHTTPContent:
		a.readContent(ctx, v.Data, true, v.Trailers)
	default:
		ctx.FireChannelRead(msg)
	}
}

func (a *HTTPObjectAggregator) ChannelInactive(ctx *channel.HandlerContext) {
	a.release()
	ctx.FireChannelInactive()
}

func (a *HTTPObjectAggregator) readRequest(ctx *channel.HandlerContext, req Request) {
	if a.kind != httpAggregateNone {
		req.Release()
		ctx.FireExceptionCaught(codec.ErrInvalidFrameLength)
		return
	}
	if req.Body != nil {
		ctx.FireChannelRead(req)
		return
	}
	a.kind = httpAggregateRequest
	a.request = req
}

func (a *HTTPObjectAggregator) readResponse(ctx *channel.HandlerContext, resp Response) {
	if a.kind != httpAggregateNone {
		resp.Release()
		ctx.FireExceptionCaught(codec.ErrInvalidFrameLength)
		return
	}
	if resp.Body != nil {
		ctx.FireChannelRead(resp)
		return
	}
	a.kind = httpAggregateResponse
	a.response = resp
}

func (a *HTTPObjectAggregator) readContent(ctx *channel.HandlerContext, data buffer.ByteBuf, last bool, trailers Headers) {
	if a.kind == httpAggregateNone {
		if data != nil {
			data.Release()
		}
		ctx.FireExceptionCaught(codec.ErrInvalidFrameLength)
		return
	}
	if data != nil && data.ReadableBytes() > 0 {
		if err := a.ensureContentLimit(data.ReadableBytes()); err != nil {
			data.Release()
			a.release()
			ctx.FireExceptionCaught(err)
			return
		}
		if a.body == nil && last {
			a.body = data
			data = nil
		} else if err := a.appendBody(ctx, data); err != nil {
			data.Release()
			ctx.FireExceptionCaught(err)
			return
		}
	}
	if data != nil {
		data.Release()
	}
	if !last {
		return
	}
	a.fireAggregated(ctx, trailers)
}

func (a *HTTPObjectAggregator) ensureContentLimit(incoming int) error {
	nextSize := incoming
	if a.body != nil {
		nextSize += a.body.ReadableBytes()
	}
	if a.maxContentBytes >= 0 && nextSize > a.maxContentBytes {
		return codec.ErrFrameTooLong
	}
	return nil
}

func (a *HTTPObjectAggregator) appendBody(ctx *channel.HandlerContext, data buffer.ByteBuf) error {
	nextSize := data.ReadableBytes()
	if a.body != nil {
		nextSize += a.body.ReadableBytes()
	}
	if a.maxContentBytes >= 0 && nextSize > a.maxContentBytes {
		a.release()
		return codec.ErrFrameTooLong
	}
	if a.body == nil {
		next, err := ctx.Channel().Allocator().Acquire(data.ReadableBytes())
		if err != nil {
			a.release()
			return err
		}
		a.body = next
	} else if a.body.WritableBytes() < data.ReadableBytes() {
		next, err := ctx.Channel().Allocator().Acquire(nextSize)
		if err != nil {
			a.release()
			return err
		}
		if err := buffer.WriteReadableBytes(next, a.body); err != nil {
			next.Release()
			a.release()
			return err
		}
		a.body.Release()
		a.body = next
	}
	err := buffer.WriteReadableBytes(a.body, data)
	if err != nil {
		a.release()
	}
	return err
}

func (a *HTTPObjectAggregator) fireAggregated(ctx *channel.HandlerContext, trailers Headers) {
	body := a.body
	a.body = nil
	switch a.kind {
	case httpAggregateRequest:
		req := a.request
		req.Body = body
		req.Headers = finalizeAggregatedHeaders(req.Headers, body, trailers)
		a.request = Request{}
		a.kind = httpAggregateNone
		ctx.FireChannelRead(req)
	case httpAggregateResponse:
		resp := a.response
		resp.Body = body
		resp.Headers = finalizeAggregatedHeaders(resp.Headers, body, trailers)
		a.response = Response{}
		a.kind = httpAggregateNone
		ctx.FireChannelRead(resp)
	default:
		if body != nil {
			body.Release()
		}
	}
}

func finalizeAggregatedHeaders(headers Headers, body buffer.ByteBuf, trailers Headers) Headers {
	if headers == nil {
		headers = Headers{}
	}
	for k, v := range trailers {
		headers.Set(k, v)
	}
	headers.Del("Transfer-Encoding")
	if body != nil {
		headers.Set("Content-Length", strconv.Itoa(body.ReadableBytes()))
	} else {
		headers.Set("Content-Length", "0")
	}
	return headers
}

func (a *HTTPObjectAggregator) release() {
	switch a.kind {
	case httpAggregateRequest:
		a.request.Release()
		a.request = Request{}
	case httpAggregateResponse:
		a.response.Release()
		a.response = Response{}
	}
	if a.body != nil {
		a.body.Release()
		a.body = nil
	}
	a.kind = httpAggregateNone
}

func findHeaderEndFrom(in *buffer.CompositeByteBuf, start int) (int, bool) {
	return in.Index(start, headerEndBytes)
}

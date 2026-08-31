package http1

import "goark.dev/gnalloy/channel"

// ContentCompressor 根据最近请求的 Accept-Encoding 压缩完整响应 body。
type ContentCompressor struct {
	minBytes int
	codings  []ContentCoding
	accepted ContentCoding
}

func NewContentCompressor(minBytes int, codings ...ContentCoding) *ContentCompressor {
	if minBytes < 0 {
		minBytes = 0
	}
	if len(codings) == 0 {
		codings = []ContentCoding{ContentCodingGzip, ContentCodingDeflate}
	}
	return &ContentCompressor{minBytes: minBytes, codings: normalizeContentCodings(codings)}
}

func (h *ContentCompressor) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if req, ok := msg.(Request); ok {
		h.accepted = chooseContentCoding(req.Headers.Get("Accept-Encoding"), h.codings)
	}
	ctx.FireChannelRead(msg)
}

func (h *ContentCompressor) ChannelInactive(ctx *channel.HandlerContext) {
	h.accepted = ""
	ctx.FireChannelInactive()
}

func (h *ContentCompressor) Write(ctx *channel.HandlerContext, msg any) error {
	resp, ok := msg.(Response)
	if !ok {
		return ctx.Write(msg)
	}
	if !canCompressResponse(resp, h.accepted, h.minBytes) {
		return ctx.Write(msg)
	}
	body, err := encodeContent(ctx, resp.Body, h.accepted)
	if err != nil {
		resp.Release()
		return err
	}
	resp.Body.Release()
	resp.Body = body
	resp.Headers = setKnownContentLength(resp.Headers, body.ReadableBytes())
	resp.Headers.Del("Content-Encoding")
	resp.Headers.Set("Content-Encoding", string(h.accepted))
	resp.Headers = setHeaderToken(resp.Headers, "Vary", "Accept-Encoding")
	return ctx.Write(resp)
}

func canCompressResponse(resp Response, coding ContentCoding, minBytes int) bool {
	if coding == "" || resp.Body == nil || resp.Body.ReadableBytes() == 0 || resp.Body.ReadableBytes() < minBytes {
		return false
	}
	if resp.Headers.Get("Content-Encoding") != "" || resp.Headers.ContainsToken("Transfer-Encoding", "chunked") {
		return false
	}
	return resp.StatusCode < 100 || (resp.StatusCode >= 200 && resp.StatusCode != 204 && resp.StatusCode != 304)
}

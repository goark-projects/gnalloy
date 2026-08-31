package http1

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

// ContentDecompressor 解压完整 Request/Response body，并更新 HTTP 头。
type ContentDecompressor struct {
	maxDecodedBytes int
}

func NewContentDecompressor(maxDecodedBytes int) *ContentDecompressor {
	if maxDecodedBytes <= 0 {
		maxDecodedBytes = DefaultMaxDecodedContentBytes
	}
	return &ContentDecompressor{maxDecodedBytes: maxDecodedBytes}
}

func (h *ContentDecompressor) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch v := msg.(type) {
	case Request:
		req, err := h.decompressRequest(ctx, v)
		if err != nil {
			v.Release()
			ctx.FireExceptionCaught(err)
			return
		}
		ctx.FireChannelRead(req)
	case Response:
		resp, err := h.decompressResponse(ctx, v)
		if err != nil {
			v.Release()
			ctx.FireExceptionCaught(err)
			return
		}
		ctx.FireChannelRead(resp)
	default:
		ctx.FireChannelRead(msg)
	}
}

func (h *ContentDecompressor) decompressRequest(ctx *channel.HandlerContext, req Request) (Request, error) {
	body, encoding, err := h.decodeBody(ctx, req.Headers, req.Body)
	if err != nil || encoding == "" {
		return req, err
	}
	req.Body.Release()
	req.Body = body
	req.Headers = setKnownContentLength(req.Headers, readableBytes(body))
	req.Headers.Del("Content-Encoding")
	return req, nil
}

func (h *ContentDecompressor) decompressResponse(ctx *channel.HandlerContext, resp Response) (Response, error) {
	body, encoding, err := h.decodeBody(ctx, resp.Headers, resp.Body)
	if err != nil || encoding == "" {
		return resp, err
	}
	resp.Body.Release()
	resp.Body = body
	resp.Headers = setKnownContentLength(resp.Headers, readableBytes(body))
	resp.Headers.Del("Content-Encoding")
	return resp, nil
}

func (h *ContentDecompressor) decodeBody(ctx *channel.HandlerContext, headers Headers, body buffer.ByteBuf) (buffer.ByteBuf, ContentCoding, error) {
	encoding := normalizeContentCoding(headers.Get("Content-Encoding"))
	if encoding == "" || encoding == "identity" || body == nil {
		return body, "", nil
	}
	if !isSupportedContentCoding(encoding) {
		return body, "", nil
	}
	decoded, err := decodeContent(body, encoding, h.maxDecodedBytes)
	if err != nil {
		return nil, encoding, err
	}
	out, err := byteBufFromBytes(ctx, decoded)
	if err != nil {
		return nil, encoding, err
	}
	return out, encoding, nil
}

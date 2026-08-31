package http1bridge

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/codec/http2"
)

// Config 描述 HTTP/2 stream frame 与 HTTP 对象桥接策略。
type Config struct {
	// Server 表示入站 HEADERS 按请求头解析；false 表示按响应头解析。
	Server bool
	// StreamID 是出站对象所属 HTTP/2 stream。
	StreamID http2.StreamID
}

// PushPromise 表示 HTTP/2 PUSH_PROMISE 对应的请求对象语义。
type PushPromise struct {
	StreamID         http2.StreamID
	PromisedStreamID http2.StreamID
	Request          http1.Request
}

// Release 释放 push promise 中可能携带的请求正文。
func (p PushPromise) Release() {
	p.Request.Release()
}

// StreamFrameToHTTPObjectCodec 在 HTTP/2 stream frame 和 HTTP 对象流之间桥接。
type StreamFrameToHTTPObjectCodec struct {
	cfg     Config
	reading bool
}

// NewStreamFrameToHTTPObjectCodec 创建 HTTP/2 stream frame 到 HTTP 对象的桥接 handler。
func NewStreamFrameToHTTPObjectCodec(cfg Config) *StreamFrameToHTTPObjectCodec {
	return &StreamFrameToHTTPObjectCodec{cfg: cfg}
}

// ChannelRead 将 HTTP/2 HEADERS/DATA/PUSH_PROMISE 转换为 HTTP 对象流。
func (c *StreamFrameToHTTPObjectCodec) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch frame := msg.(type) {
	case http2.HeadersBlock:
		c.readHeaders(ctx, frame)
	case http2.DataFrame:
		c.readData(ctx, frame)
	case http2.PushPromiseBlock:
		c.readPushPromise(ctx, frame)
	default:
		ctx.FireChannelRead(msg)
	}
}

// ChannelInactive 在异常关闭时释放桥接状态，不制造协议层不存在的 END_STREAM。
func (c *StreamFrameToHTTPObjectCodec) ChannelInactive(ctx *channel.HandlerContext) {
	c.reading = false
	ctx.FireChannelInactive()
}

// Write 将 HTTP 请求、响应和内容对象转换为 HTTP/2 HEADERS/DATA。
func (c *StreamFrameToHTTPObjectCodec) Write(ctx *channel.HandlerContext, msg any) error {
	switch obj := msg.(type) {
	case http1.Request:
		return c.writeRequest(ctx, obj)
	case http1.Response:
		return c.writeResponse(ctx, obj)
	case http1.HTTPContent:
		return c.writeDataFrame(ctx, obj.Data, 0)
	case http1.LastHTTPContent:
		return c.writeLastContent(ctx, obj)
	default:
		return ctx.Write(msg)
	}
}

func (c *StreamFrameToHTTPObjectCodec) readHeaders(ctx *channel.HandlerContext, block http2.HeadersBlock) {
	if c.reading {
		c.readTrailers(ctx, block)
		return
	}
	if c.cfg.Server {
		req, err := RequestFromHeadersBlock(block)
		if err != nil {
			ctx.FireExceptionCaught(err)
			return
		}
		c.reading = !block.EndStream
		ctx.FireChannelRead(req)
		c.fireEmptyLastIfNeeded(ctx, block.EndStream)
		return
	}
	resp, err := ResponseFromHeadersBlock(block)
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	c.reading = !block.EndStream
	ctx.FireChannelRead(resp)
	c.fireEmptyLastIfNeeded(ctx, block.EndStream)
}

func (c *StreamFrameToHTTPObjectCodec) readTrailers(ctx *channel.HandlerContext, block http2.HeadersBlock) {
	if !block.EndStream {
		ctx.FireExceptionCaught(codec.ErrInvalidFrameLength)
		return
	}
	trailers, err := trailersFromHeadersBlock(block)
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	c.reading = false
	ctx.FireChannelRead(http1.LastHTTPContent{Trailers: trailers})
}

func (c *StreamFrameToHTTPObjectCodec) readData(ctx *channel.HandlerContext, frame http2.DataFrame) {
	if !c.reading {
		frame.Release()
		ctx.FireExceptionCaught(codec.ErrInvalidFrameLength)
		return
	}
	if frame.Flags&http2.FlagEndStream != 0 {
		c.reading = false
		ctx.FireChannelRead(http1.LastHTTPContent{Data: frame.Data})
		return
	}
	ctx.FireChannelRead(http1.HTTPContent{Data: frame.Data})
}

func (c *StreamFrameToHTTPObjectCodec) readPushPromise(ctx *channel.HandlerContext, block http2.PushPromiseBlock) {
	req, err := RequestFromHeadersBlock(http2.HeadersBlock{StreamID: block.PromisedStreamID, Fields: block.Fields})
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(PushPromise{StreamID: block.StreamID, PromisedStreamID: block.PromisedStreamID, Request: req})
}

func (c *StreamFrameToHTTPObjectCodec) writeRequest(ctx *channel.HandlerContext, req http1.Request) error {
	body := req.Body
	req.Body = nil
	if err := ctx.Write(HeadersBlockFromRequest(c.streamID(), req, body == nil)); err != nil {
		if body != nil {
			body.Release()
		}
		return err
	}
	return c.writeDataFrame(ctx, body, http2.FlagEndStream)
}

func (c *StreamFrameToHTTPObjectCodec) writeResponse(ctx *channel.HandlerContext, resp http1.Response) error {
	body := resp.Body
	resp.Body = nil
	if err := ctx.Write(HeadersBlockFromResponse(c.streamID(), resp, body == nil)); err != nil {
		if body != nil {
			body.Release()
		}
		return err
	}
	return c.writeDataFrame(ctx, body, http2.FlagEndStream)
}

func (c *StreamFrameToHTTPObjectCodec) writeLastContent(ctx *channel.HandlerContext, last http1.LastHTTPContent) error {
	if len(last.Trailers) == 0 {
		return c.writeDataFrame(ctx, last.Data, http2.FlagEndStream)
	}
	if last.Data != nil {
		if err := c.writeDataFrame(ctx, last.Data, 0); err != nil {
			return err
		}
	}
	return ctx.Write(HeadersBlockFromTrailers(c.streamID(), last.Trailers, true))
}

func (c *StreamFrameToHTTPObjectCodec) writeDataFrame(ctx *channel.HandlerContext, data buffer.ByteBuf, flags http2.Flags) error {
	if data == nil {
		if flags&http2.FlagEndStream == 0 {
			return nil
		}
		return ctx.Write(http2.DataFrame{StreamID: c.streamID(), Flags: flags})
	}
	frame := http2.DataFrame{StreamID: c.streamID(), Flags: flags, Data: data}
	if err := ctx.Write(frame); err != nil {
		frame.Release()
		return err
	}
	return nil
}

func (c *StreamFrameToHTTPObjectCodec) fireEmptyLastIfNeeded(ctx *channel.HandlerContext, endStream bool) {
	if endStream {
		ctx.FireChannelRead(http1.LastHTTPContent{})
	}
}

func (c *StreamFrameToHTTPObjectCodec) streamID() http2.StreamID {
	return c.cfg.StreamID
}

package http1bridge

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/codec/http3"
)

// Config 描述 HTTP/3 frame 与 HTTP 对象桥接策略。
type Config struct {
	// Server 表示入站 HEADERS 按请求头解析；false 表示按响应头解析。
	Server bool
	// Scheme 是出站请求缺省 :scheme；为空时使用 https。
	Scheme string
}

// FrameToHTTPObjectCodec 在 HTTP/3 frame 语义和 HTTP/1 对象流之间桥接。
type FrameToHTTPObjectCodec struct {
	cfg     Config
	reading bool
}

// NewFrameToHTTPObjectCodec 创建 HTTP/3 HTTP 对象桥接 handler。
func NewFrameToHTTPObjectCodec(cfg Config) *FrameToHTTPObjectCodec {
	if cfg.Scheme == "" {
		cfg.Scheme = defaultScheme
	}
	return &FrameToHTTPObjectCodec{cfg: cfg}
}

// ChannelRead 将 HTTP/3 HEADERS/DATA/PUSH_PROMISE 转换为 HTTP 对象流。
func (c *FrameToHTTPObjectCodec) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch frame := msg.(type) {
	case http3.HeadersBlock:
		c.readHeaders(ctx, frame)
	case http3.DataFrame:
		c.readData(ctx, frame)
	case http3.PushPromiseBlock:
		c.readPushPromise(ctx, frame)
	default:
		ctx.FireChannelRead(msg)
	}
}

// ChannelInactive 在 QUIC stream EOF 时补齐对象流的 LastHTTPContent。
func (c *FrameToHTTPObjectCodec) ChannelInactive(ctx *channel.HandlerContext) {
	if c.reading {
		c.reading = false
		ctx.FireChannelRead(http1.LastHTTPContent{})
	}
	ctx.FireChannelInactive()
}

// Write 将 HTTP 请求、响应和内容对象转换为 HTTP/3 HEADERS/DATA。
func (c *FrameToHTTPObjectCodec) Write(ctx *channel.HandlerContext, msg any) error {
	switch obj := msg.(type) {
	case http1.Request:
		return c.writeRequest(ctx, obj)
	case http1.Response:
		return c.writeResponse(ctx, obj)
	case http1.HTTPContent:
		return c.writeContent(ctx, obj.Data)
	case http1.LastHTTPContent:
		return c.writeLastContent(ctx, obj)
	default:
		return ctx.Write(msg)
	}
}

func (c *FrameToHTTPObjectCodec) readHeaders(ctx *channel.HandlerContext, block http3.HeadersBlock) {
	if c.reading {
		trailers, err := TrailersFromHeadersBlock(block)
		if err != nil {
			ctx.FireExceptionCaught(err)
			return
		}
		c.reading = false
		ctx.FireChannelRead(http1.LastHTTPContent{Trailers: trailers})
		return
	}
	if c.cfg.Server {
		req, err := RequestFromHeadersBlock(block)
		if err != nil {
			ctx.FireExceptionCaught(err)
			return
		}
		c.reading = true
		ctx.FireChannelRead(req)
		return
	}
	resp, err := ResponseFromHeadersBlock(block)
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	c.reading = true
	ctx.FireChannelRead(resp)
}

func (c *FrameToHTTPObjectCodec) readData(ctx *channel.HandlerContext, frame http3.DataFrame) {
	if !c.reading {
		frame.Release()
		ctx.FireExceptionCaught(codec.ErrInvalidFrameLength)
		return
	}
	ctx.FireChannelRead(http1.HTTPContent{Data: frame.Data})
}

func (c *FrameToHTTPObjectCodec) readPushPromise(ctx *channel.HandlerContext, block http3.PushPromiseBlock) {
	promise, err := PushPromiseFromBlock(block)
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(promise)
}

func (c *FrameToHTTPObjectCodec) writeRequest(ctx *channel.HandlerContext, req http1.Request) error {
	body := req.Body
	req.Body = nil
	if err := ctx.Write(HeadersBlockFromRequest(req, c.cfg.Scheme)); err != nil {
		if body != nil {
			body.Release()
		}
		return err
	}
	return c.writeDataFrame(ctx, body)
}

func (c *FrameToHTTPObjectCodec) writeResponse(ctx *channel.HandlerContext, resp http1.Response) error {
	body := resp.Body
	resp.Body = nil
	if err := ctx.Write(HeadersBlockFromResponse(resp)); err != nil {
		if body != nil {
			body.Release()
		}
		return err
	}
	return c.writeDataFrame(ctx, body)
}

func (c *FrameToHTTPObjectCodec) writeContent(ctx *channel.HandlerContext, data buffer.ByteBuf) error {
	return c.writeDataFrame(ctx, data)
}

func (c *FrameToHTTPObjectCodec) writeLastContent(ctx *channel.HandlerContext, last http1.LastHTTPContent) error {
	if err := c.writeDataFrame(ctx, last.Data); err != nil {
		return err
	}
	if len(last.Trailers) == 0 {
		return nil
	}
	return ctx.Write(HeadersBlockFromTrailers(last.Trailers))
}

func (c *FrameToHTTPObjectCodec) writeDataFrame(ctx *channel.HandlerContext, data buffer.ByteBuf) error {
	if data == nil {
		return nil
	}
	frame := http3.DataFrame{Data: data}
	if err := ctx.Write(frame); err != nil {
		frame.Release()
		return err
	}
	return nil
}

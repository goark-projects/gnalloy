package content

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/http2"
)

// ResponseCompressorConfig 描述 HTTP/2 响应压缩策略。
type ResponseCompressorConfig struct {
	// MinBytes 是启用压缩的最小明文字节数。
	MinBytes int
	// Codings 按优先级列出可用编码，空值使用 gzip、deflate。
	Codings []Coding
}

type responseState struct {
	headers http2.HeadersBlock
	coding  Coding
	body    *buffer.CompositeByteBuf
}

// ResponseCompressor 根据同 stream 请求的 accept-encoding 压缩响应 DATA。
type ResponseCompressor struct {
	minBytes int
	codings  []Coding
	accepted map[http2.StreamID]Coding
	pending  map[http2.StreamID]*responseState
}

// NewResponseCompressor 创建 HTTP/2 响应压缩 handler。
func NewResponseCompressor(cfg ResponseCompressorConfig) *ResponseCompressor {
	minBytes := cfg.MinBytes
	if minBytes < 0 {
		minBytes = 0
	}
	return &ResponseCompressor{minBytes: minBytes, codings: normalizeCodings(cfg.Codings)}
}

// ChannelRead 观察请求 HEADERS 中的 accept-encoding。
func (c *ResponseCompressor) ChannelRead(ctx *channel.HandlerContext, msg any) {
	headers, ok := msg.(http2.HeadersBlock)
	if ok {
		c.readRequestHeaders(headers)
	}
	ctx.FireChannelRead(msg)
}

// ChannelInactive 释放未完成的响应体聚合状态。
func (c *ResponseCompressor) ChannelInactive(ctx *channel.HandlerContext) {
	c.release()
	ctx.FireChannelInactive()
}

// Write 对出站响应 HEADERS/DATA 应用 Content-Encoding。
func (c *ResponseCompressor) Write(ctx *channel.HandlerContext, msg any) error {
	switch frame := msg.(type) {
	case http2.HeadersBlock:
		return c.writeHeaders(ctx, frame)
	case http2.DataFrame:
		return c.writeData(ctx, frame)
	default:
		return ctx.Write(msg)
	}
}

func (c *ResponseCompressor) readRequestHeaders(headers http2.HeadersBlock) {
	if getHeader(headers.Fields, ":method") == "" {
		return
	}
	coding := chooseCoding(getHeader(headers.Fields, "accept-encoding"), c.codings)
	if coding == "" {
		delete(c.accepted, headers.StreamID)
		return
	}
	if c.accepted == nil {
		c.accepted = make(map[http2.StreamID]Coding, 4)
	}
	c.accepted[headers.StreamID] = coding
}

func (c *ResponseCompressor) writeHeaders(ctx *channel.HandlerContext, frame http2.HeadersBlock) error {
	if state := c.response(frame.StreamID); state != nil {
		if err := c.finish(ctx, frame.StreamID, false); err != nil {
			return err
		}
		return ctx.Write(frame)
	}
	coding := c.accepted[frame.StreamID]
	if coding == "" || frame.EndStream || !canCompressResponseHeaders(frame.Fields) {
		return ctx.Write(frame)
	}
	if c.pending == nil {
		c.pending = make(map[http2.StreamID]*responseState, 4)
	}
	c.pending[frame.StreamID] = &responseState{
		headers: http2.HeadersBlock{
			StreamID:  frame.StreamID,
			Fields:    cloneFields(frame.Fields),
			EndStream: frame.EndStream,
			Priority:  frame.Priority,
			Padding:   frame.Padding,
		},
		coding: coding,
		body:   buffer.NewCompositeByteBuf(),
	}
	return nil
}

func (c *ResponseCompressor) writeData(ctx *channel.HandlerContext, frame http2.DataFrame) error {
	state := c.response(frame.StreamID)
	if state == nil {
		return ctx.Write(frame)
	}
	if frame.Data != nil {
		state.body.Append(frame.Data)
		frame.Data = nil
	}
	if frame.Flags&http2.FlagEndStream == 0 {
		return nil
	}
	return c.finish(ctx, frame.StreamID, true)
}

func (c *ResponseCompressor) finish(ctx *channel.HandlerContext, streamID http2.StreamID, endStream bool) error {
	state := c.response(streamID)
	if state == nil {
		return nil
	}
	delete(c.pending, streamID)
	delete(c.accepted, streamID)
	if state.body == nil || state.body.ReadableBytes() < c.minBytes {
		return c.writePlain(ctx, state, endStream)
	}
	encoded, err := encodeBody(ctx, state.body, state.coding)
	state.body.Release()
	state.body = nil
	if err != nil {
		if encoded != nil {
			encoded.Release()
		}
		return err
	}
	headers := state.headers
	headers.EndStream = false
	headers.Fields = removeHeaders(cloneFields(headers.Fields), "content-length", "content-encoding")
	headers.Fields = setHeader(headers.Fields, "content-encoding", string(state.coding))
	headers.Fields = addHeaderToken(headers.Fields, "vary", "accept-encoding")
	if err := ctx.Write(headers); err != nil {
		if encoded != nil {
			encoded.Release()
		}
		return err
	}
	flags := http2.Flags(0)
	if endStream {
		flags = http2.FlagEndStream
	}
	frame := http2.DataFrame{StreamID: streamID, Flags: flags, Data: encoded}
	if err := ctx.Write(frame); err != nil {
		frame.Release()
		return err
	}
	return nil
}

func (c *ResponseCompressor) writePlain(ctx *channel.HandlerContext, state *responseState, endStream bool) error {
	headers := state.headers
	headers.EndStream = state.body == nil || state.body.ReadableBytes() == 0
	if err := ctx.Write(headers); err != nil {
		if state.body != nil {
			state.body.Release()
			state.body = nil
		}
		return err
	}
	if state.body == nil || state.body.ReadableBytes() == 0 {
		if state.body != nil {
			state.body.Release()
			state.body = nil
		}
		return nil
	}
	flags := http2.Flags(0)
	if endStream {
		flags = http2.FlagEndStream
	}
	frame := http2.DataFrame{StreamID: headers.StreamID, Flags: flags, Data: state.body}
	state.body = nil
	if err := ctx.Write(frame); err != nil {
		frame.Release()
		return err
	}
	return nil
}

func (c *ResponseCompressor) response(streamID http2.StreamID) *responseState {
	if c == nil || c.pending == nil {
		return nil
	}
	return c.pending[streamID]
}

func (c *ResponseCompressor) release() {
	for streamID, state := range c.pending {
		if state != nil && state.body != nil {
			state.body.Release()
		}
		delete(c.pending, streamID)
	}
	for streamID := range c.accepted {
		delete(c.accepted, streamID)
	}
}

func canCompressResponseHeaders(fields []http2.HeaderField) bool {
	return getHeader(fields, "content-encoding") == "" && responseCanHaveBody(fields)
}

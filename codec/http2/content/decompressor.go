package content

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/http2"
)

type decompressState struct {
	coding Coding
	body   *buffer.CompositeByteBuf
}

// Decompressor 解压 HTTP/2 DATA 流，并同步更新 header block。
type Decompressor struct {
	maxDecodedBytes int
	streams         map[http2.StreamID]*decompressState
}

// NewDecompressor 创建 HTTP/2 content decompressor。
func NewDecompressor(maxDecodedBytes int) *Decompressor {
	if maxDecodedBytes <= 0 {
		maxDecodedBytes = defaultMaxDecodedBytes
	}
	return &Decompressor{maxDecodedBytes: maxDecodedBytes}
}

// ChannelRead 对入站 HEADERS/DATA 应用 Content-Encoding 解压。
func (d *Decompressor) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch frame := msg.(type) {
	case http2.HeadersBlock:
		d.readHeaders(ctx, frame)
	case http2.DataFrame:
		d.readData(ctx, frame)
	default:
		ctx.FireChannelRead(msg)
	}
}

// ChannelInactive 释放未完成的压缩状态。
func (d *Decompressor) ChannelInactive(ctx *channel.HandlerContext) {
	d.release()
	ctx.FireChannelInactive()
}

func (d *Decompressor) readHeaders(ctx *channel.HandlerContext, frame http2.HeadersBlock) {
	state := d.state(frame.StreamID)
	if state != nil {
		if frame.EndStream {
			if err := d.finish(ctx, frame.StreamID, false); err != nil {
				ctx.FireExceptionCaught(err)
				return
			}
		}
		ctx.FireChannelRead(frame)
		return
	}
	coding := normalizeCoding(getHeader(frame.Fields, "content-encoding"))
	if coding == "" || coding == "identity" || !isSupportedCoding(coding) {
		ctx.FireChannelRead(frame)
		return
	}
	frame.Fields = removeHeaders(cloneFields(frame.Fields), "content-encoding", "content-length")
	if frame.EndStream {
		ctx.FireChannelRead(frame)
		return
	}
	if d.streams == nil {
		d.streams = make(map[http2.StreamID]*decompressState, 4)
	}
	d.streams[frame.StreamID] = &decompressState{coding: coding, body: buffer.NewCompositeByteBuf()}
	ctx.FireChannelRead(frame)
}

func (d *Decompressor) readData(ctx *channel.HandlerContext, frame http2.DataFrame) {
	state := d.state(frame.StreamID)
	if state == nil {
		ctx.FireChannelRead(frame)
		return
	}
	if frame.Data != nil {
		state.body.Append(frame.Data)
		frame.Data = nil
	}
	if frame.Flags&http2.FlagEndStream == 0 {
		return
	}
	if err := d.finish(ctx, frame.StreamID, true); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (d *Decompressor) finish(ctx *channel.HandlerContext, streamID http2.StreamID, endStream bool) error {
	state := d.state(streamID)
	if state == nil {
		return nil
	}
	delete(d.streams, streamID)
	decoded, err := decodeBody(ctx, state.body, state.coding, d.maxDecodedBytes)
	state.body.Release()
	if err != nil {
		if decoded != nil {
			decoded.Release()
		}
		return err
	}
	flags := http2.Flags(0)
	if endStream {
		flags = http2.FlagEndStream
	}
	ctx.FireChannelRead(http2.DataFrame{StreamID: streamID, Flags: flags, Data: decoded})
	return nil
}

func (d *Decompressor) state(streamID http2.StreamID) *decompressState {
	if d == nil || d.streams == nil {
		return nil
	}
	return d.streams[streamID]
}

func (d *Decompressor) release() {
	for streamID, state := range d.streams {
		if state != nil && state.body != nil {
			state.body.Release()
		}
		delete(d.streams, streamID)
	}
}

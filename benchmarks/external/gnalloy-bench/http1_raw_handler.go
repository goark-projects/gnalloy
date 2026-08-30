package main

import (
	"goark.dev/gnalloy/benchmarks/external/internal/benchhttp"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

// http1RawHandler 执行 HTTP/1 keep-alive 固定响应快路径。
//
// 该 handler 只用于外部性能对标：它复用 Gnalloy transport/pipeline/TLS，但避免通用
// Request/Headers/Response 对象编解码，使场景与 gnet/netpoll 的轻量 HTTP/1 实现对齐。
type http1RawHandler struct {
	state    benchhttp.ServerState
	response []byte
}

func newHTTP1RawHandler(payload int) *http1RawHandler {
	return &http1RawHandler{response: benchhttp.ResponseBytes(payload)}
}

func (h *http1RawHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	count, alive := h.countRequests(buf)
	buf.Release()
	if !alive {
		ctx.FireExceptionCaught(buffer.ErrReleasedBuffer)
		return
	}
	if count == 0 {
		return
	}
	if err := ctx.WriteStaticBytesAndFlush(h.responseBytes(count)); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
}

func (http1RawHandler) ExceptionCaught(ctx *channel.HandlerContext, _ error) {
	_ = ctx.Close()
}

func (h *http1RawHandler) countRequests(buf buffer.ByteBuf) (int, bool) {
	if data, ok := buffer.ContiguousReadableBytes(buf); ok {
		return h.state.AppendAndCountRequests(data), true
	}
	count := 0
	ok := buffer.ForEachReadableSlice(buf, func(part []byte) bool {
		count += h.state.AppendAndCountRequests(part)
		return true
	})
	return count, ok
}

func (h *http1RawHandler) responseBytes(count int) []byte {
	if count <= 1 {
		return h.response
	}
	total := len(h.response) * count
	data := make([]byte, 0, total)
	for i := 0; i < count; i++ {
		data = append(data, h.response...)
	}
	return data
}

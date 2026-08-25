package websocket

import (
	"crypto/sha1"
	"encoding/base64"
	"strings"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/http1"
)

const acceptGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type HandshakeComplete struct {
	Method  string
	URI     string
	Headers http1.Headers
}

type ServerHandshakeHandler struct {
	path           string
	removeHandlers []string
}

func NewServerHandshakeHandler(path string, removeHandlers ...string) *ServerHandshakeHandler {
	return &ServerHandshakeHandler{path: path, removeHandlers: removeHandlers}
}

func AcceptKey(key string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(key) + acceptGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func IsUpgradeRequest(req http1.Request) bool {
	return strings.EqualFold(req.Method, "GET") &&
		req.Headers.ContainsToken("Connection", "Upgrade") &&
		strings.EqualFold(req.Headers.Get("Upgrade"), "websocket") &&
		strings.TrimSpace(req.Headers.Get("Sec-WebSocket-Key")) != "" &&
		strings.TrimSpace(req.Headers.Get("Sec-WebSocket-Version")) == "13"
}

func (h *ServerHandshakeHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	req, ok := msg.(http1.Request)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if h.path != "" && req.URI != h.path {
		ctx.FireChannelRead(req)
		return
	}
	if !IsUpgradeRequest(req) {
		req.Release()
		_ = ctx.Channel().WriteAndFlush(http1.Response{StatusCode: 400})
		_ = ctx.Close()
		return
	}
	resp := http1.Response{
		StatusCode: 101,
		Headers: http1.Headers{
			"Upgrade":              "websocket",
			"Connection":           "Upgrade",
			"Sec-WebSocket-Accept": AcceptKey(req.Headers.Get("Sec-WebSocket-Key")),
		},
	}
	if err := ctx.Channel().WriteAndFlush(resp); err != nil {
		req.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireUserEventTriggered(HandshakeComplete{
		Method:  req.Method,
		URI:     req.URI,
		Headers: cloneHeaders(req.Headers),
	})
	for _, name := range h.removeHandlers {
		if name == "" {
			continue
		}
		if err := ctx.Pipeline().Remove(name); err != nil {
			req.Release()
			ctx.FireExceptionCaught(err)
			return
		}
	}
	req.Release()
}

func cloneHeaders(headers http1.Headers) http1.Headers {
	out := make(http1.Headers, len(headers))
	for k, v := range headers {
		out[k] = v
	}
	return out
}

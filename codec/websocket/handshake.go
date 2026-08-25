package websocket

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"net/url"
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

// ClientHandshakeConfig 描述客户端 WebSocket 升级请求参数。
type ClientHandshakeConfig struct {
	URL            string
	Host           string
	Headers        http1.Headers
	RemoveHandlers []string
}

type ServerHandshakeHandler struct {
	path           string
	removeHandlers []string
}

// ClientHandshakeHandler 负责客户端 HTTP Upgrade 请求和服务端握手响应校验。
type ClientHandshakeHandler struct {
	host           string
	uri            string
	key            string
	headers        http1.Headers
	removeHandlers []string
}

func NewServerHandshakeHandler(path string, removeHandlers ...string) *ServerHandshakeHandler {
	return &ServerHandshakeHandler{path: path, removeHandlers: removeHandlers}
}

func NewClientHandshakeHandler(config ClientHandshakeConfig) (*ClientHandshakeHandler, error) {
	host, uri, err := parseClientHandshakeTarget(config.URL, config.Host)
	if err != nil {
		return nil, err
	}
	key, err := newClientKey()
	if err != nil {
		return nil, err
	}
	return &ClientHandshakeHandler{
		host:           host,
		uri:            uri,
		key:            key,
		headers:        cloneHeaders(config.Headers),
		removeHandlers: append([]string(nil), config.RemoveHandlers...),
	}, nil
}

func AcceptKey(key string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(key) + acceptGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func (h *ClientHandshakeHandler) Key() string {
	return h.key
}

func IsUpgradeRequest(req http1.Request) bool {
	return strings.EqualFold(req.Method, "GET") &&
		req.Headers.ContainsToken("Connection", "Upgrade") &&
		strings.EqualFold(req.Headers.Get("Upgrade"), "websocket") &&
		strings.TrimSpace(req.Headers.Get("Sec-WebSocket-Key")) != "" &&
		strings.TrimSpace(req.Headers.Get("Sec-WebSocket-Version")) == "13"
}

func (h *ClientHandshakeHandler) ChannelActive(ctx *channel.HandlerContext) {
	req := http1.Request{
		Method:  "GET",
		URI:     h.uri,
		Version: "HTTP/1.1",
		Headers: h.requestHeaders(),
	}
	if err := ctx.Channel().WriteAndFlush(req); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelActive()
}

func (h *ClientHandshakeHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	resp, ok := msg.(http1.Response)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if !h.isUpgradeResponse(resp) {
		resp.Release()
		ctx.FireExceptionCaught(ErrInvalidHandshake)
		_ = ctx.Close()
		return
	}
	ctx.FireUserEventTriggered(HandshakeComplete{
		Method:  "GET",
		URI:     h.uri,
		Headers: cloneHeaders(resp.Headers),
	})
	for _, name := range h.removeHandlers {
		if name == "" {
			continue
		}
		if err := ctx.Pipeline().Remove(name); err != nil {
			resp.Release()
			ctx.FireExceptionCaught(err)
			return
		}
	}
	resp.Release()
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

func (h *ClientHandshakeHandler) requestHeaders() http1.Headers {
	headers := cloneHeaders(h.headers)
	headers.Set("Host", h.host)
	headers.Set("Upgrade", "websocket")
	headers.Set("Connection", "Upgrade")
	headers.Set("Sec-WebSocket-Key", h.key)
	headers.Set("Sec-WebSocket-Version", "13")
	return headers
}

func (h *ClientHandshakeHandler) isUpgradeResponse(resp http1.Response) bool {
	return resp.StatusCode == 101 &&
		strings.EqualFold(resp.Headers.Get("Upgrade"), "websocket") &&
		resp.Headers.ContainsToken("Connection", "Upgrade") &&
		strings.TrimSpace(resp.Headers.Get("Sec-WebSocket-Accept")) == AcceptKey(h.key)
}

func parseClientHandshakeTarget(rawURL string, explicitHost string) (string, string, error) {
	if rawURL == "" {
		return "", "", ErrInvalidHandshake
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", ErrInvalidHandshake
	}
	host := explicitHost
	if host == "" {
		host = u.Host
	}
	if host == "" {
		return "", "", ErrInvalidHandshake
	}
	if u.Scheme != "" && !strings.EqualFold(u.Scheme, "ws") && !strings.EqualFold(u.Scheme, "wss") {
		return "", "", ErrInvalidHandshake
	}
	uri := u.RequestURI()
	if uri == "" {
		uri = "/"
	}
	return host, uri, nil
}

func newClientKey() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw[:]), nil
}

func cloneHeaders(headers http1.Headers) http1.Headers {
	out := make(http1.Headers, len(headers))
	for k, v := range headers {
		out[k] = v
	}
	return out
}

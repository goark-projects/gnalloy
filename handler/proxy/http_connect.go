package proxy

import (
	"strconv"
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

type Headers map[string]string

type HTTPConnectRequest struct {
	Target  string
	Headers Headers
}

type HTTPConnectResponse struct {
	Version    string
	StatusCode int
	Reason     string
	Headers    Headers
}

type HTTPConnectEvent struct {
	Response HTTPConnectResponse
}

func AppendHTTPConnectRequest(dst []byte, req HTTPConnectRequest) ([]byte, error) {
	if req.Target == "" {
		return nil, ErrInvalidMessage
	}
	var b strings.Builder
	b.WriteString("CONNECT ")
	b.WriteString(req.Target)
	b.WriteString(" HTTP/1.1\r\n")
	hasHost := false
	for name, value := range req.Headers {
		if strings.EqualFold(name, "Host") {
			hasHost = true
		}
		b.WriteString(name)
		b.WriteString(": ")
		b.WriteString(value)
		b.WriteString("\r\n")
	}
	if !hasHost {
		b.WriteString("Host: ")
		b.WriteString(req.Target)
		b.WriteString("\r\n")
	}
	b.WriteString("\r\n")
	return append(dst, b.String()...), nil
}

func ParseHTTPConnectResponse(data []byte) (HTTPConnectResponse, int, error) {
	end := headerEnd(data)
	if end < 0 {
		return HTTPConnectResponse{}, 0, ErrNeedMore
	}
	head := string(data[:end])
	lines := strings.Split(head, "\r\n")
	if len(lines) == 0 {
		return HTTPConnectResponse{}, 0, ErrInvalidMessage
	}
	parts := strings.SplitN(lines[0], " ", 3)
	if len(parts) < 2 {
		return HTTPConnectResponse{}, 0, ErrInvalidMessage
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return HTTPConnectResponse{}, 0, ErrInvalidMessage
	}
	reason := ""
	if len(parts) == 3 {
		reason = parts[2]
	}
	resp := HTTPConnectResponse{
		Version:    parts[0],
		StatusCode: code,
		Reason:     reason,
		Headers:    make(Headers, len(lines)),
	}
	for _, line := range lines[1:] {
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return HTTPConnectResponse{}, 0, ErrInvalidMessage
		}
		resp.Headers[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	return resp, end + 4, nil
}

// HTTPConnectClient 在 Channel 激活后发起 HTTP CONNECT 握手。
type HTTPConnectClient struct {
	req      HTTPConnectRequest
	complete bool
}

func NewHTTPConnectClient(req HTTPConnectRequest) (*HTTPConnectClient, error) {
	if req.Target == "" {
		return nil, ErrInvalidMessage
	}
	return &HTTPConnectClient{req: req}, nil
}

func (h *HTTPConnectClient) ChannelActive(ctx *channel.HandlerContext) {
	payload, err := AppendHTTPConnectRequest(nil, h.req)
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if err := writeProxyPayload(ctx, payload); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelActive()
}

func (h *HTTPConnectClient) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if h.complete {
		ctx.FireChannelRead(msg)
		return
	}
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	resp, consumed, err := ParseHTTPConnectResponse(buf.Bytes())
	if err != nil {
		buf.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		buf.Release()
		ctx.FireExceptionCaught(ErrHandshakeFailed)
		return
	}
	h.complete = true
	ctx.FireUserEventTriggered(HTTPConnectEvent{Response: resp})
	if remaining := buf.ReadableBytes() - consumed; remaining > 0 {
		next, err := ctx.Channel().Allocator().Acquire(remaining)
		if err != nil {
			buf.Release()
			ctx.FireExceptionCaught(err)
			return
		}
		if _, err := next.WriteBytes(buf.Bytes()[consumed:]); err != nil {
			next.Release()
			buf.Release()
			ctx.FireExceptionCaught(err)
			return
		}
		ctx.FireChannelRead(next)
	}
	buf.Release()
}

func headerEnd(data []byte) int {
	for i := 0; i+3 < len(data); i++ {
		if data[i] == '\r' && data[i+1] == '\n' && data[i+2] == '\r' && data[i+3] == '\n' {
			return i
		}
	}
	return -1
}

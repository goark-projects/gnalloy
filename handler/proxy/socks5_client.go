package proxy

import (
	"errors"
	"fmt"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

const maxSOCKS5HandshakeBytes = 512

type socks5ClientState uint8

const (
	socks5StateGreeting socks5ClientState = iota
	socks5StateAuth
	socks5StateConnect
	socks5StateComplete
)

// SOCKS5Event 表示 SOCKS5 代理握手成功。
type SOCKS5Event struct {
	Method byte
	Reply  SOCKS5Reply
}

// SOCKS5UsernamePassword 保存 RFC1929 用户名密码认证材料。
type SOCKS5UsernamePassword struct {
	Username string
	Password string
}

// SOCKS5Client 在 Channel 激活后执行 SOCKS5 CONNECT 握手。
type SOCKS5Client struct {
	target  string
	methods []byte
	auth    *SOCKS5UsernamePassword
	state   socks5ClientState
	pending []byte
	method  byte
}

// NewSOCKS5Client 创建 SOCKS5 CONNECT client handler。
func NewSOCKS5Client(target string, methods ...byte) (*SOCKS5Client, error) {
	if len(methods) == 0 {
		methods = []byte{SOCKS5MethodNoAuth}
	}
	return newSOCKS5Client(target, nil, methods)
}

// NewSOCKS5UsernamePasswordClient 创建带 username/password 子协商的 CONNECT client handler。
func NewSOCKS5UsernamePasswordClient(target string, username string, password string, methods ...byte) (*SOCKS5Client, error) {
	if len(methods) == 0 {
		methods = []byte{SOCKS5MethodUserPassword}
	}
	auth := &SOCKS5UsernamePassword{Username: username, Password: password}
	return newSOCKS5Client(target, auth, methods)
}

func newSOCKS5Client(target string, auth *SOCKS5UsernamePassword, methods []byte) (*SOCKS5Client, error) {
	if _, err := AppendSOCKS5Greeting(nil, methods...); err != nil {
		return nil, err
	}
	if auth != nil {
		if !containsMethod(methods, SOCKS5MethodUserPassword) {
			return nil, ErrInvalidMessage
		}
		if _, err := AppendSOCKS5UsernamePasswordAuth(nil, auth.Username, auth.Password); err != nil {
			return nil, err
		}
	}
	if _, err := AppendSOCKS5Connect(nil, target); err != nil {
		return nil, err
	}
	return &SOCKS5Client{
		target:  target,
		methods: append([]byte(nil), methods...),
		auth:    cloneSOCKS5UsernamePassword(auth),
	}, nil
}

func (h *SOCKS5Client) ChannelActive(ctx *channel.HandlerContext) {
	if err := h.writeGreeting(ctx); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelActive()
}

func (h *SOCKS5Client) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if h == nil || h.state == socks5StateComplete {
		ctx.FireChannelRead(msg)
		return
	}
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	h.pending = append(h.pending, buf.Bytes()...)
	buf.Release()
	if len(h.pending) > maxSOCKS5HandshakeBytes {
		ctx.FireExceptionCaught(ErrInvalidMessage)
		return
	}
	if err := h.advance(ctx); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (h *SOCKS5Client) advance(ctx *channel.HandlerContext) error {
	for h.state != socks5StateComplete {
		var err error
		switch h.state {
		case socks5StateGreeting:
			err = h.readGreeting(ctx)
		case socks5StateAuth:
			err = h.readAuth(ctx)
		case socks5StateConnect:
			err = h.readConnectReply(ctx)
		default:
			return ErrInvalidMessage
		}
		if errors.Is(err, ErrNeedMore) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *SOCKS5Client) readGreeting(ctx *channel.HandlerContext) error {
	method, consumed, err := ParseSOCKS5GreetingResponse(h.pending)
	if errors.Is(err, ErrNeedMore) {
		return ErrNeedMore
	}
	if err != nil {
		return err
	}
	if !h.methodAccepted(method) {
		return fmt.Errorf("%w: socks5 method 0x%02x", ErrHandshakeFailed, method)
	}
	h.method = method
	h.pending = h.pending[consumed:]
	if method == SOCKS5MethodUserPassword {
		if h.auth == nil {
			return fmt.Errorf("%w: socks5 username/password credentials missing", ErrHandshakeFailed)
		}
		h.state = socks5StateAuth
		return h.writeAuth(ctx)
	}
	h.state = socks5StateConnect
	return h.writeConnect(ctx)
}

func (h *SOCKS5Client) readAuth(ctx *channel.HandlerContext) error {
	status, consumed, err := ParseSOCKS5UsernamePasswordAuthResponse(h.pending)
	if errors.Is(err, ErrNeedMore) {
		return ErrNeedMore
	}
	if err != nil {
		return err
	}
	if status != SOCKS5AuthStatusSuccess {
		return fmt.Errorf("%w: socks5 username/password status 0x%02x", ErrHandshakeFailed, status)
	}
	h.pending = h.pending[consumed:]
	h.state = socks5StateConnect
	return h.writeConnect(ctx)
}

func (h *SOCKS5Client) readConnectReply(ctx *channel.HandlerContext) error {
	reply, consumed, err := ParseSOCKS5Reply(h.pending)
	if errors.Is(err, ErrNeedMore) {
		return ErrNeedMore
	}
	if err != nil {
		return err
	}
	if reply.Status != 0 {
		return fmt.Errorf("%w: socks5 status 0x%02x", ErrHandshakeFailed, reply.Status)
	}
	h.pending = h.pending[consumed:]
	h.state = socks5StateComplete
	ctx.FireUserEventTriggered(SOCKS5Event{Method: h.method, Reply: reply})
	return h.fireRemaining(ctx)
}

func (h *SOCKS5Client) methodAccepted(method byte) bool {
	for _, offered := range h.methods {
		if offered == method {
			return true
		}
	}
	return false
}

func (h *SOCKS5Client) writeGreeting(ctx *channel.HandlerContext) error {
	payload, err := AppendSOCKS5Greeting(nil, h.methods...)
	if err != nil {
		return err
	}
	return writeProxyPayload(ctx, payload)
}

func (h *SOCKS5Client) writeConnect(ctx *channel.HandlerContext) error {
	payload, err := AppendSOCKS5Connect(nil, h.target)
	if err != nil {
		return err
	}
	return writeProxyPayload(ctx, payload)
}

func (h *SOCKS5Client) writeAuth(ctx *channel.HandlerContext) error {
	payload, err := AppendSOCKS5UsernamePasswordAuth(nil, h.auth.Username, h.auth.Password)
	if err != nil {
		return err
	}
	return writeProxyPayload(ctx, payload)
}

func (h *SOCKS5Client) fireRemaining(ctx *channel.HandlerContext) error {
	if len(h.pending) == 0 {
		h.pending = nil
		return nil
	}
	out, err := ctx.Channel().Allocator().Acquire(len(h.pending))
	if err != nil {
		return err
	}
	if _, err := out.WriteBytes(h.pending); err != nil {
		out.Release()
		return err
	}
	h.pending = nil
	ctx.FireChannelRead(out)
	return nil
}

func containsMethod(methods []byte, method byte) bool {
	for _, offered := range methods {
		if offered == method {
			return true
		}
	}
	return false
}

func cloneSOCKS5UsernamePassword(auth *SOCKS5UsernamePassword) *SOCKS5UsernamePassword {
	if auth == nil {
		return nil
	}
	clone := *auth
	return &clone
}

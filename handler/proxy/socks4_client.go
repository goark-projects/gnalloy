package proxy

import (
	"fmt"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

const maxSOCKS4HandshakeBytes = 64

// SOCKS4Event 表示 SOCKS4 代理握手成功。
type SOCKS4Event struct {
	Reply SOCKS4Reply
}

// SOCKS4Client 在 Channel 激活后执行 SOCKS4 CONNECT 握手。
type SOCKS4Client struct {
	target   string
	userID   string
	pending  []byte
	complete bool
}

// NewSOCKS4Client 创建 SOCKS4/SOCKS4a CONNECT client handler。
func NewSOCKS4Client(target string, userID string) (*SOCKS4Client, error) {
	if _, err := AppendSOCKS4Connect(nil, target, userID); err != nil {
		return nil, err
	}
	return &SOCKS4Client{target: target, userID: userID}, nil
}

func (h *SOCKS4Client) ChannelActive(ctx *channel.HandlerContext) {
	payload, err := AppendSOCKS4Connect(nil, h.target, h.userID)
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

func (h *SOCKS4Client) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if h == nil || h.complete {
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
	if len(h.pending) > maxSOCKS4HandshakeBytes {
		ctx.FireExceptionCaught(ErrInvalidMessage)
		return
	}
	if err := h.advance(ctx); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (h *SOCKS4Client) advance(ctx *channel.HandlerContext) error {
	reply, consumed, err := ParseSOCKS4Reply(h.pending)
	if err != nil {
		if err == ErrNeedMore {
			return nil
		}
		return err
	}
	if reply.Status != SOCKS4StatusGranted {
		return fmt.Errorf("%w: socks4 status 0x%02x", ErrHandshakeFailed, reply.Status)
	}
	h.pending = h.pending[consumed:]
	h.complete = true
	ctx.FireUserEventTriggered(SOCKS4Event{Reply: reply})
	return h.fireRemaining(ctx)
}

func (h *SOCKS4Client) fireRemaining(ctx *channel.HandlerContext) error {
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

package icmp

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/raw"
)

type EchoResponder struct{}

func NewEchoResponder() *EchoResponder {
	return &EchoResponder{}
}

func (h *EchoResponder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	addressed, ok := asRawAddressed(msg)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	request, ok := addressed.Message.(*Message)
	if !ok || !request.IsEchoRequest() {
		ctx.FireChannelRead(msg)
		return
	}
	protocol := addressed.Protocol
	if protocol == 0 {
		protocol = request.Protocol
	}
	reply := NewEchoReply(protocol, request.Identifier, request.Sequence, request.Payload)
	request.Payload = nil
	addressed.Release()
	if err := ctx.Channel().WriteAndFlush(raw.Addressed{Message: reply, Addr: addressed.Addr, Protocol: protocol}); err != nil {
		reply.Release()
		ctx.FireExceptionCaught(err)
	}
}

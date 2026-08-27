package protocol

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

type response struct {
	payload []byte
	err     error
}

type collector struct {
	adapter   Adapter
	request   []byte
	responses chan response
}

func newCollector(adapter Adapter, request []byte) *collector {
	return &collector{
		adapter:   adapter,
		request:   append([]byte(nil), request...),
		responses: make(chan response, 1),
	}
}

func (c *collector) ChannelRead(_ *channel.HandlerContext, msg any) {
	defer releaseMessage(msg)
	payload, matched, err := c.adapter.MatchResponse(c.request, msg)
	if !matched && err == nil {
		return
	}
	select {
	case c.responses <- response{payload: payload, err: err}:
	default:
	}
}

func (c *collector) ChannelInactive(*channel.HandlerContext) {
	select {
	case c.responses <- response{err: ErrNoResponse}:
	default:
	}
}

func (c *collector) ExceptionCaught(_ *channel.HandlerContext, err error) {
	select {
	case c.responses <- response{err: err}:
	default:
	}
}

func releaseMessage(msg any) {
	if msg == nil {
		return
	}
	if buf, ok := msg.(buffer.ByteBuf); ok {
		buf.Release()
		return
	}
	if releasable, ok := msg.(interface{ Release() }); ok {
		releasable.Release()
	}
}

package channel

type writeAndFlushSink interface {
	WriteAndFlush(msg any) error
}

type HandlerContext struct {
	pipeline *Pipeline
	name     string
	handler  Handler
	prev     *HandlerContext
	next     *HandlerContext

	channelRead         ChannelReadHandler
	channelReadComplete ChannelReadCompleteHandler
	exceptionCaught     ExceptionCaughtHandler
	write               WriteHandler
	flush               FlushHandler
}

func (c *HandlerContext) Name() string {
	return c.name
}

func (c *HandlerContext) Channel() Channel {
	return c.pipeline.ch
}

func (c *HandlerContext) Pipeline() *Pipeline {
	return c.pipeline
}

func (c *HandlerContext) Handler() Handler {
	return c.handler
}

func (c *HandlerContext) FireChannelRegistered() {
	for n := c.next; n != nil; n = n.next {
		if h, ok := n.handler.(ChannelRegisteredHandler); ok {
			h.ChannelRegistered(n)
			return
		}
	}
}

func (c *HandlerContext) FireChannelUnregistered() {
	for n := c.next; n != nil; n = n.next {
		if h, ok := n.handler.(ChannelUnregisteredHandler); ok {
			h.ChannelUnregistered(n)
			return
		}
	}
}

func (c *HandlerContext) FireChannelActive() {
	for n := c.next; n != nil; n = n.next {
		if h, ok := n.handler.(ChannelActiveHandler); ok {
			h.ChannelActive(n)
			return
		}
	}
}

func (c *HandlerContext) FireChannelRead(msg any) {
	for n := c.next; n != nil; n = n.next {
		if n.channelRead != nil {
			n.channelRead.ChannelRead(n, msg)
			return
		}
	}
}

func (c *HandlerContext) FireChannelReadComplete() {
	for n := c.next; n != nil; n = n.next {
		if n.channelReadComplete != nil {
			n.channelReadComplete.ChannelReadComplete(n)
			return
		}
	}
}

func (c *HandlerContext) FireChannelInactive() {
	for n := c.next; n != nil; n = n.next {
		if h, ok := n.handler.(ChannelInactiveHandler); ok {
			h.ChannelInactive(n)
			return
		}
	}
}

func (c *HandlerContext) FireChannelWritabilityChanged() {
	for n := c.next; n != nil; n = n.next {
		if h, ok := n.handler.(ChannelWritabilityChangedHandler); ok {
			h.ChannelWritabilityChanged(n)
			return
		}
	}
}

func (c *HandlerContext) FireUserEventTriggered(event any) {
	for n := c.next; n != nil; n = n.next {
		if h, ok := n.handler.(UserEventTriggeredHandler); ok {
			h.UserEventTriggered(n, event)
			return
		}
	}
}

func (c *HandlerContext) FireFlushComplete() {
	for n := c.next; n != nil; n = n.next {
		if h, ok := n.handler.(FlushCompleteHandler); ok {
			h.FlushComplete(n)
			return
		}
	}
}

func (c *HandlerContext) FireExceptionCaught(err error) {
	for n := c.next; n != nil; n = n.next {
		if n.exceptionCaught != nil {
			n.exceptionCaught.ExceptionCaught(n, err)
			return
		}
	}
}

func (c *HandlerContext) WriteFuture(msg any) Future {
	return writeFutureFrom(c, msg)
}

func (c *HandlerContext) Write(msg any) error {
	for n := c.prev; n != nil; n = n.prev {
		if n.write != nil {
			return n.write.Write(n, msg)
		}
	}
	if c.pipeline.sink == nil {
		return ErrNoOutboundSink
	}
	return c.pipeline.sink.Write(msg)
}

func (c *HandlerContext) FlushFuture() Future {
	return flushFutureFrom(c)
}

func (c *HandlerContext) Flush() error {
	for n := c.prev; n != nil; n = n.prev {
		if n.flush != nil {
			return n.flush.Flush(n)
		}
	}
	if c.pipeline.sink == nil {
		return ErrNoOutboundSink
	}
	return c.pipeline.sink.Flush()
}

func (c *HandlerContext) WriteAndFlush(msg any) error {
	if sink, ok := c.directWriteAndFlushSink(); ok {
		return sink.WriteAndFlush(msg)
	}
	if err := c.Write(msg); err != nil {
		return err
	}
	return c.Flush()
}

func (c *HandlerContext) directWriteAndFlushSink() (writeAndFlushSink, bool) {
	if c == nil || c.pipeline == nil || c.pipeline.writeAndFlush == nil {
		return nil, false
	}
	if c.pipeline.outboundHandlers != 0 {
		return nil, false
	}
	return c.pipeline.writeAndFlush, true
}

func (c *HandlerContext) CloseFuture() Future {
	return closeFutureFrom(c)
}

func (c *HandlerContext) Close() error {
	for n := c.prev; n != nil; n = n.prev {
		if h, ok := n.handler.(CloseHandler); ok {
			return h.Close(n)
		}
	}
	if c.pipeline.sink == nil {
		return ErrNoOutboundSink
	}
	return c.pipeline.sink.Close()
}

package channel

// Handler 是 Pipeline 中的处理器标记类型。
type Handler any

type ChannelActiveHandler interface {
	ChannelActive(ctx *HandlerContext)
}

type ChannelReadHandler interface {
	ChannelRead(ctx *HandlerContext, msg any)
}

type ChannelInactiveHandler interface {
	ChannelInactive(ctx *HandlerContext)
}

type ChannelWritabilityChangedHandler interface {
	ChannelWritabilityChanged(ctx *HandlerContext)
}

type ExceptionCaughtHandler interface {
	ExceptionCaught(ctx *HandlerContext, err error)
}

type WriteHandler interface {
	Write(ctx *HandlerContext, msg any) error
}

type FlushHandler interface {
	Flush(ctx *HandlerContext) error
}

type CloseHandler interface {
	Close(ctx *HandlerContext) error
}

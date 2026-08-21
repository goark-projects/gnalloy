package channel

import "goark.dev/gnalloy/buffer"

type Pipeline struct {
	ch    Channel
	sink  OutboundSink
	head  *HandlerContext
	tail  *HandlerContext
	names map[string]*HandlerContext
}

func NewPipeline(ch Channel, sink OutboundSink) *Pipeline {
	p := &Pipeline{
		ch:    ch,
		sink:  sink,
		names: make(map[string]*HandlerContext, 8),
	}
	p.head = &HandlerContext{pipeline: p, name: "$head", handler: headHandler{}}
	p.tail = &HandlerContext{pipeline: p, name: "$tail", handler: tailHandler{}}
	p.head.next = p.tail
	p.tail.prev = p.head
	return p
}

func (p *Pipeline) Channel() Channel {
	return p.ch
}

func (p *Pipeline) AddLast(name string, h Handler) error {
	if name == "" || h == nil {
		return ErrHandlerNotFound
	}
	if _, exists := p.names[name]; exists {
		return ErrDuplicateHandler
	}
	ctx := &HandlerContext{pipeline: p, name: name, handler: h}
	prev := p.tail.prev
	prev.next = ctx
	ctx.prev = prev
	ctx.next = p.tail
	p.tail.prev = ctx
	p.names[name] = ctx
	return nil
}

func (p *Pipeline) AddFirst(name string, h Handler) error {
	if name == "" || h == nil {
		return ErrHandlerNotFound
	}
	if _, exists := p.names[name]; exists {
		return ErrDuplicateHandler
	}
	ctx := &HandlerContext{pipeline: p, name: name, handler: h}
	next := p.head.next
	p.head.next = ctx
	ctx.prev = p.head
	ctx.next = next
	next.prev = ctx
	p.names[name] = ctx
	return nil
}

func (p *Pipeline) Remove(name string) error {
	ctx, ok := p.names[name]
	if !ok {
		return ErrHandlerNotFound
	}
	ctx.prev.next = ctx.next
	ctx.next.prev = ctx.prev
	ctx.prev = nil
	ctx.next = nil
	delete(p.names, name)
	return nil
}

func (p *Pipeline) Context(name string) (*HandlerContext, bool) {
	ctx, ok := p.names[name]
	return ctx, ok
}

func (p *Pipeline) FireChannelActive() {
	p.head.FireChannelActive()
}

func (p *Pipeline) FireChannelRead(msg any) {
	p.head.FireChannelRead(msg)
}

func (p *Pipeline) FireChannelInactive() {
	p.head.FireChannelInactive()
}

func (p *Pipeline) FireChannelWritabilityChanged() {
	p.head.FireChannelWritabilityChanged()
}

func (p *Pipeline) FireExceptionCaught(err error) {
	p.head.FireExceptionCaught(err)
}

func (p *Pipeline) Write(msg any) error {
	return p.tail.Write(msg)
}

func (p *Pipeline) Flush() error {
	return p.tail.Flush()
}

func (p *Pipeline) Close() error {
	return p.tail.Close()
}

type headHandler struct{}

type tailHandler struct{}

func (tailHandler) ChannelRead(_ *HandlerContext, msg any) {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		buf.Release()
	}
}

func (tailHandler) ExceptionCaught(_ *HandlerContext, _ error) {}

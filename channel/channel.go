package channel

import (
	"github.com/goark-projects/gnalloy/buffer"
	"github.com/goark-projects/gnalloy/transport"
)

// Channel 是业务层看到的连接抽象，不暴露 fd、CQE 等平台细节。
type Channel interface {
	ID() transport.ChannelID
	Pipeline() *Pipeline
	Allocator() buffer.Allocator
}

// OutboundSink 是出站事件穿过 Pipeline 后的最终写出端。
type OutboundSink interface {
	Write(msg any) error
	Flush() error
	Close() error
}

type LocalChannel struct {
	id       transport.ChannelID
	pipeline *Pipeline
	alloc    buffer.Allocator
}

func NewLocalChannel(id transport.ChannelID, alloc buffer.Allocator, sink OutboundSink) *LocalChannel {
	ch := &LocalChannel{id: id, alloc: alloc}
	ch.pipeline = NewPipeline(ch, sink)
	return ch
}

func (c *LocalChannel) ID() transport.ChannelID {
	return c.id
}

func (c *LocalChannel) Pipeline() *Pipeline {
	return c.pipeline
}

func (c *LocalChannel) Allocator() buffer.Allocator {
	return c.alloc
}

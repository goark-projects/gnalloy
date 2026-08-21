package channel

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

// Channel 是业务层看到的连接抽象，不暴露 fd、CQE 等平台细节。
type Channel interface {
	ID() transport.ChannelID
	Pipeline() *Pipeline
	Allocator() buffer.Allocator
	Write(msg any) error
	Flush() error
	WriteAndFlush(msg any) error
	IsWritable() bool
}

// OutboundSink 是出站事件穿过 Pipeline 后的最终写出端。
type OutboundSink interface {
	Write(msg any) error
	Flush() error
	Close() error
}

type WritabilitySink interface {
	IsWritable() bool
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

func (c *LocalChannel) Write(msg any) error {
	return c.pipeline.Write(msg)
}

func (c *LocalChannel) Flush() error {
	return c.pipeline.Flush()
}

func (c *LocalChannel) WriteAndFlush(msg any) error {
	if err := c.Write(msg); err != nil {
		return err
	}
	return c.Flush()
}

func (c *LocalChannel) IsWritable() bool {
	if sink, ok := c.pipeline.sink.(WritabilitySink); ok {
		return sink.IsWritable()
	}
	return false
}

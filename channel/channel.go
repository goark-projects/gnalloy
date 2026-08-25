package channel

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/timer"
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
	WriteFuture(msg any) Future
	FlushFuture() Future
	WriteAndFlushFuture(msg any) Future
	CloseFuture() Future
	IsWritable() bool
	ScheduleTimer(delayMillis int64, cb timer.Callback) (*timer.Task, error)
	CancelTimer(task *timer.Task) bool
}

// OutboundSink 是出站事件穿过 Pipeline 后的最终写出端。
type OutboundSink interface {
	Write(msg any) error
	Flush() error
	Close() error
}

type FutureOutboundSink interface {
	WriteFuture(msg any) Future
	FlushFuture() Future
	CloseFuture() Future
}

type WritabilitySink interface {
	IsWritable() bool
}

type LocalChannel struct {
	id       transport.ChannelID
	pipeline *Pipeline
	alloc    buffer.Allocator
	timer    *timer.Wheel
}

func NewLocalChannel(id transport.ChannelID, alloc buffer.Allocator, sink OutboundSink) *LocalChannel {
	return NewLocalChannelWithTimer(id, alloc, sink, nil)
}

func NewLocalChannelWithTimer(id transport.ChannelID, alloc buffer.Allocator, sink OutboundSink, wheel *timer.Wheel) *LocalChannel {
	ch := &LocalChannel{id: id, alloc: alloc, timer: wheel}
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

func (c *LocalChannel) WriteFuture(msg any) Future {
	return c.pipeline.WriteFuture(msg)
}

func (c *LocalChannel) Flush() error {
	return c.pipeline.Flush()
}

func (c *LocalChannel) FlushFuture() Future {
	return c.pipeline.FlushFuture()
}

func (c *LocalChannel) WriteAndFlush(msg any) error {
	if err := c.Write(msg); err != nil {
		return err
	}
	return c.Flush()
}

func (c *LocalChannel) WriteAndFlushFuture(msg any) Future {
	return c.pipeline.WriteAndFlushFuture(msg)
}

func (c *LocalChannel) CloseFuture() Future {
	return c.pipeline.CloseFuture()
}

func (c *LocalChannel) IsWritable() bool {
	if sink, ok := c.pipeline.sink.(WritabilitySink); ok {
		return sink.IsWritable()
	}
	return false
}

func (c *LocalChannel) ScheduleTimer(delayMillis int64, cb timer.Callback) (*timer.Task, error) {
	if c.timer == nil {
		return nil, ErrNoTimer
	}
	return c.timer.Schedule(delayMillis, cb)
}

func (c *LocalChannel) CancelTimer(task *timer.Task) bool {
	if c.timer == nil {
		return false
	}
	return c.timer.Cancel(task)
}

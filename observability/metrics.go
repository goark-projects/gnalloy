package observability

import (
	"sync/atomic"

	"goark.dev/gnalloy/transport"
)

// ChannelMetrics 是 Channel 维度指标的快照。
//
// 字段全部是聚合值，不包含连接 ID、远端地址、错误文本等高基数字段，避免把
// 运行时观测变成指标后端的基数压力。
type ChannelMetrics struct {
	RegisteredChannels   uint64
	UnregisteredChannels uint64
	ActiveTransitions    uint64
	InactiveTransitions  uint64
	ActiveChannels       int64
	InboundMessages      uint64
	InboundBytes         uint64
	InboundCompletions   uint64
	OutboundMessages     uint64
	OutboundBytes        uint64
	Flushes              uint64
	Closes               uint64
	Exceptions           uint64
}

// ChannelRecorder 接收 Pipeline 生命周期、读写和异常事件。
//
// 实现方必须保证并发安全；事件处理位于 I/O 热路径，不能做阻塞 I/O 或无界
// 分配。ChannelID 只用于外部实现做受控关联，默认聚合器不会按 ID 拆分指标。
type ChannelRecorder interface {
	RecordChannelRegistered(id transport.ChannelID)
	RecordChannelUnregistered(id transport.ChannelID)
	RecordChannelActive(id transport.ChannelID)
	RecordChannelInactive(id transport.ChannelID)
	RecordChannelRead(id transport.ChannelID, bytes int64)
	RecordChannelReadComplete(id transport.ChannelID)
	RecordChannelWrite(id transport.ChannelID, bytes int64)
	RecordChannelFlush(id transport.ChannelID)
	RecordChannelClose(id transport.ChannelID)
	RecordException(id transport.ChannelID, err error)
}

// Snapshotter 表示可导出聚合指标快照的记录器。
type Snapshotter interface {
	Snapshot() ChannelMetrics
}

// NoopChannelRecorder 是默认空实现，用于关闭观测或测试中占位。
type NoopChannelRecorder struct{}

func (NoopChannelRecorder) RecordChannelRegistered(transport.ChannelID)   {}
func (NoopChannelRecorder) RecordChannelUnregistered(transport.ChannelID) {}
func (NoopChannelRecorder) RecordChannelActive(transport.ChannelID)       {}
func (NoopChannelRecorder) RecordChannelInactive(transport.ChannelID)     {}
func (NoopChannelRecorder) RecordChannelRead(transport.ChannelID, int64)  {}
func (NoopChannelRecorder) RecordChannelReadComplete(transport.ChannelID) {}
func (NoopChannelRecorder) RecordChannelWrite(transport.ChannelID, int64) {}
func (NoopChannelRecorder) RecordChannelFlush(transport.ChannelID)        {}
func (NoopChannelRecorder) RecordChannelClose(transport.ChannelID)        {}
func (NoopChannelRecorder) RecordException(transport.ChannelID, error)    {}

// AtomicChannelRecorder 使用原子计数器记录 Channel 聚合指标。
//
// 它适合单进程 smoke、压测和嵌入式导出场景。需要标签、直方图或分布式追踪时，
// 应实现 ChannelRecorder 并在 handler/metrics 中替换。
type AtomicChannelRecorder struct {
	registeredChannels   atomic.Uint64
	unregisteredChannels atomic.Uint64
	activeTransitions    atomic.Uint64
	inactiveTransitions  atomic.Uint64
	activeChannels       atomic.Int64
	inboundMessages      atomic.Uint64
	inboundBytes         atomic.Uint64
	inboundCompletions   atomic.Uint64
	outboundMessages     atomic.Uint64
	outboundBytes        atomic.Uint64
	flushes              atomic.Uint64
	closes               atomic.Uint64
	exceptions           atomic.Uint64
}

func NewAtomicChannelRecorder() *AtomicChannelRecorder {
	return &AtomicChannelRecorder{}
}

func (r *AtomicChannelRecorder) RecordChannelRegistered(transport.ChannelID) {
	r.registeredChannels.Add(1)
}

func (r *AtomicChannelRecorder) RecordChannelUnregistered(transport.ChannelID) {
	r.unregisteredChannels.Add(1)
}

func (r *AtomicChannelRecorder) RecordChannelActive(transport.ChannelID) {
	r.activeTransitions.Add(1)
	r.activeChannels.Add(1)
}

func (r *AtomicChannelRecorder) RecordChannelInactive(transport.ChannelID) {
	r.inactiveTransitions.Add(1)
	r.activeChannels.Add(-1)
}

func (r *AtomicChannelRecorder) RecordChannelRead(_ transport.ChannelID, bytes int64) {
	r.inboundMessages.Add(1)
	addNonNegative(&r.inboundBytes, bytes)
}

func (r *AtomicChannelRecorder) RecordChannelReadComplete(transport.ChannelID) {
	r.inboundCompletions.Add(1)
}

func (r *AtomicChannelRecorder) RecordChannelWrite(_ transport.ChannelID, bytes int64) {
	r.outboundMessages.Add(1)
	addNonNegative(&r.outboundBytes, bytes)
}

func (r *AtomicChannelRecorder) RecordChannelFlush(transport.ChannelID) {
	r.flushes.Add(1)
}

func (r *AtomicChannelRecorder) RecordChannelClose(transport.ChannelID) {
	r.closes.Add(1)
}

func (r *AtomicChannelRecorder) RecordException(transport.ChannelID, error) {
	r.exceptions.Add(1)
}

func (r *AtomicChannelRecorder) Snapshot() ChannelMetrics {
	return ChannelMetrics{
		RegisteredChannels:   r.registeredChannels.Load(),
		UnregisteredChannels: r.unregisteredChannels.Load(),
		ActiveTransitions:    r.activeTransitions.Load(),
		InactiveTransitions:  r.inactiveTransitions.Load(),
		ActiveChannels:       r.activeChannels.Load(),
		InboundMessages:      r.inboundMessages.Load(),
		InboundBytes:         r.inboundBytes.Load(),
		InboundCompletions:   r.inboundCompletions.Load(),
		OutboundMessages:     r.outboundMessages.Load(),
		OutboundBytes:        r.outboundBytes.Load(),
		Flushes:              r.flushes.Load(),
		Closes:               r.closes.Load(),
		Exceptions:           r.exceptions.Load(),
	}
}

func addNonNegative(counter *atomic.Uint64, value int64) {
	if value > 0 {
		counter.Add(uint64(value))
	}
}

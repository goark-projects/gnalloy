package channel

import (
	"sync/atomic"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/timer"
	"goark.dev/gnalloy/transport"
)

const defaultReadBufferSize = 4096

const (
	maxGatheringBuffers = 16
)

// FDReadWriter 抽象 readiness 模型下的非阻塞 fd 读写。
// again 表示底层返回 EAGAIN/EWOULDBLOCK，调用方必须停止本轮读写。
type FDReadWriter interface {
	Read(fd transport.FDRef, dst []byte) (n int, again bool, err error)
	Write(fd transport.FDRef, src []byte) (n int, again bool, err error)
	Close(fd transport.FDRef) error
}

// FDVectorWriter 是支持 gathering write 的 readiness 后端可选能力。
type FDVectorWriter interface {
	Writev(fd transport.FDRef, src [][]byte) (n int, again bool, err error)
}

// UnsafeConfig 描述底层 I/O 分界线创建参数。
type UnsafeConfig struct {
	ID                   transport.ChannelID
	FD                   transport.FDRef
	Allocator            buffer.Allocator
	Poller               transport.Poller
	ReadWriter           FDReadWriter
	CloseHook            func(*Unsafe)
	ReadBufferSize       int
	WriteBufferWatermark transport.WriteBufferWatermark
	WriteHighWatermark   int
	WriteLowWatermark    int
	FixedBuffers         bool
	Timer                *timer.Wheel
}

// Unsafe 是底层 I/O 事件与业务 Pipeline 的分界线。
type Unsafe struct {
	ch             *LocalChannel
	fd             transport.FDRef
	poller         transport.Poller
	rw             FDReadWriter
	closeHook      func(*Unsafe)
	readBufferSize int
	fixedBuffers   bool
	registered     atomic.Bool
	closed         atomic.Bool
	inactiveFired  atomic.Bool

	outHead       *outboundEntry
	outTail       *outboundEntry
	outFree       *outboundEntry
	outboundBytes atomic.Int64
	eventExecutor atomic.Value
	readPending   bool
	writePending  bool
	writeInterest bool
	readCallback  bool
	deferredFlush bool
	writeBatch    []buffer.ByteBuf
	writeSlices   [][]byte
	flushWaiters  []*DefaultPromise
	closePromise  *DefaultPromise

	writeHighWatermark int64
	writeLowWatermark  int64
	writable           atomic.Bool
}

type outboundEntry struct {
	buf     buffer.ByteBuf
	promise Promise
	next    *outboundEntry
}

func NewUnsafeChannel(cfg UnsafeConfig) (*LocalChannel, *Unsafe) {
	readBufferSize := cfg.ReadBufferSize
	if readBufferSize <= 0 {
		readBufferSize = defaultReadBufferSize
	}
	watermark := cfg.WriteBufferWatermark
	if watermark.High == 0 && watermark.Low == 0 && (cfg.WriteHighWatermark != 0 || cfg.WriteLowWatermark != 0) {
		watermark = transport.WriteBufferWatermark{Low: cfg.WriteLowWatermark, High: cfg.WriteHighWatermark}
	}
	watermark = transport.NormalizeWriteBufferWatermark(watermark)
	u := &Unsafe{
		fd:                 cfg.FD,
		poller:             cfg.Poller,
		rw:                 cfg.ReadWriter,
		closeHook:          cfg.CloseHook,
		readBufferSize:     readBufferSize,
		fixedBuffers:       cfg.FixedBuffers,
		closePromise:       NewPromise(),
		writeHighWatermark: int64(watermark.High),
		writeLowWatermark:  int64(watermark.Low),
	}
	u.writable.Store(true)
	u.ch = NewLocalChannelWithTimer(cfg.ID, cfg.Allocator, u, cfg.Timer)
	OptionReadBufferSize.Set(u.ch.Options(), readBufferSize)
	OptionWriteBufferWatermark.Set(u.ch.Options(), watermark)
	OptionWriteSpinCount.Set(u.ch.Options(), OptionWriteSpinCount.Get(u.ch.Options()))
	OptionMaxMessagesPerRead.Set(u.ch.Options(), OptionMaxMessagesPerRead.Get(u.ch.Options()))
	return u.ch, u
}

func (u *Unsafe) ID() transport.ChannelID {
	return u.ch.ID()
}

func (u *Unsafe) FD() transport.FDRef {
	return u.fd
}

func (u *Unsafe) Channel() *LocalChannel {
	return u.ch
}

func (u *Unsafe) IsWritable() bool {
	return u.writable.Load()
}

func (u *Unsafe) PendingOutboundBytes() int64 {
	return u.outboundBytes.Load()
}

func (u *Unsafe) WriteBufferWatermark() transport.WriteBufferWatermark {
	return transport.WriteBufferWatermark{Low: int(u.writeLowWatermark), High: int(u.writeHighWatermark)}
}

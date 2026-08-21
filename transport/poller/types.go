package poller

import (
	"errors"

	"github.com/goark-projects/gnalloy/buffer"
)

var (
	ErrUnsupportedPoller = errors.New("gnalloy/transport/poller: unsupported poller")
	ErrClosedPoller      = errors.New("gnalloy/transport/poller: poller closed")
	ErrInvalidFD         = errors.New("gnalloy/transport/poller: invalid file descriptor")
	ErrInvalidIORequest  = errors.New("gnalloy/transport/poller: invalid io request")
)

type EventLoopID uint32
type EventLoopGroupID uint32
type ChannelID uint64
type OpID uint64
type TaskID uint64

// FDRef 用 generation 区分被操作系统复用的描述符。
// FD 在 Unix 表示 fd，在 Windows 表示 SOCKET/HANDLE 的整数值。
type FDRef struct {
	FD         int
	Generation uint32
}

func (f FDRef) Valid() bool {
	return f.FD >= 0
}

type Model uint8

const (
	Readiness Model = iota
	Completion
)

type IOOp uint8

const (
	OpAccept IOOp = iota
	OpRead
	OpWrite
	OpClose
	OpWakeup
)

type ReadyMask uint32

const (
	ReadyRead ReadyMask = 1 << iota
	ReadyWrite
	ReadyHangup
	ReadyError
)

type BackendKind uint8

const (
	BackendMemory BackendKind = iota
	BackendStd
	BackendEpoll
	BackendKqueue
	BackendIOUring
	BackendIOCP
)

type Event struct {
	Model     Model
	Op        IOOp
	Ready     ReadyMask
	FD        FDRef
	ChannelID ChannelID
	OpID      OpID
	Buf       buffer.ByteBuf
	N         int
	Err       error
}

type IORequest struct {
	Op        IOOp
	FD        FDRef
	ChannelID ChannelID
	OpID      OpID
	Buf       buffer.ByteBuf
	Ready     ReadyMask
}

// Poller 是所有平台 I/O 后端必须实现的最小契约。
// readiness 后端返回可读可写状态，completion 后端返回已完成的 I/O 请求。
type Poller interface {
	Model() Model
	Backend() BackendKind
	Register(fd FDRef, ch ChannelID, interest ReadyMask) error
	Modify(fd FDRef, interest ReadyMask) error
	Deregister(fd FDRef) error
	Submit(req IORequest) error
	Poll(dst []Event, timeoutMillis int) (int, error)
	Wakeup() error
	Close() error
}

type Config struct {
	Backend BackendKind
}

func CompletionReady(op IOOp) ReadyMask {
	switch op {
	case OpRead, OpAccept:
		return ReadyRead
	case OpWrite:
		return ReadyWrite
	default:
		return 0
	}
}

func ReadinessOp(ready ReadyMask) IOOp {
	if ready&(ReadyHangup|ReadyError) != 0 {
		return OpClose
	}
	if ready&ReadyRead != 0 {
		return OpRead
	}
	if ready&ReadyWrite != 0 {
		return OpWrite
	}
	return OpWakeup
}

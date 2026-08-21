package channel

import (
	"github.com/goark-projects/gnalloy/buffer"
	"github.com/goark-projects/gnalloy/transport"
)

const defaultReadBufferSize = 4096

// FDReadWriter 抽象 readiness 模型下的非阻塞 fd 读写。
// again 表示底层返回 EAGAIN/EWOULDBLOCK，调用方必须停止本轮读写。
type FDReadWriter interface {
	Read(fd transport.FDRef, dst []byte) (n int, again bool, err error)
	Write(fd transport.FDRef, src []byte) (n int, again bool, err error)
	Close(fd transport.FDRef) error
}

type UnsafeConfig struct {
	ID             transport.ChannelID
	FD             transport.FDRef
	Allocator      buffer.Allocator
	Poller         transport.Poller
	ReadWriter     FDReadWriter
	ReadBufferSize int
}

// Unsafe 是底层 I/O 事件与业务 Pipeline 的分界线。
type Unsafe struct {
	ch             *LocalChannel
	fd             transport.FDRef
	poller         transport.Poller
	rw             FDReadWriter
	readBufferSize int
	closed         bool
}

func NewUnsafeChannel(cfg UnsafeConfig) (*LocalChannel, *Unsafe) {
	readBufferSize := cfg.ReadBufferSize
	if readBufferSize <= 0 {
		readBufferSize = defaultReadBufferSize
	}
	u := &Unsafe{
		fd:             cfg.FD,
		poller:         cfg.Poller,
		rw:             cfg.ReadWriter,
		readBufferSize: readBufferSize,
	}
	u.ch = NewLocalChannel(cfg.ID, cfg.Allocator, u)
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

func (u *Unsafe) HandleEvent(ev transport.PollEvent) {
	if u.closed {
		return
	}
	if ev.Err != nil {
		u.fail(ev.Err)
		return
	}
	if ev.Model == transport.PollerCompletion {
		u.handleCompletion(ev)
		return
	}
	u.handleReadiness(ev)
}

func (u *Unsafe) Write(msg any) error {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return ErrInvalidMessage
	}
	if u.poller != nil && u.poller.Model() == transport.PollerCompletion {
		err := u.poller.Submit(transport.IORequest{
			Op:        transport.OpWrite,
			FD:        u.fd,
			ChannelID: u.ID(),
			Buf:       buf,
		})
		buf.Release()
		return err
	}
	return u.writeReady(buf)
}

func (u *Unsafe) Flush() error {
	return nil
}

func (u *Unsafe) Close() error {
	if u.closed {
		return nil
	}
	u.closed = true
	if u.rw != nil {
		return u.rw.Close(u.fd)
	}
	return nil
}

func (u *Unsafe) BeginRead() error {
	if u.poller == nil || u.poller.Model() != transport.PollerCompletion {
		return nil
	}
	return u.submitRead()
}

func (u *Unsafe) handleReadiness(ev transport.PollEvent) {
	if ev.Ready&(transport.ReadyError|transport.ReadyHangup) != 0 {
		_ = u.Close()
		u.ch.Pipeline().FireChannelInactive()
		return
	}
	if ev.Ready&transport.ReadyRead != 0 {
		u.readReady()
	}
}

func (u *Unsafe) handleCompletion(ev transport.PollEvent) {
	switch ev.Op {
	case transport.OpRead:
		if ev.N > 0 && ev.Buf != nil {
			u.ch.Pipeline().FireChannelRead(ev.Buf)
		} else if ev.Buf != nil {
			ev.Buf.Release()
		}
		if !u.closed {
			if err := u.submitRead(); err != nil {
				u.fail(err)
			}
		}
	case transport.OpWrite:
		if ev.Buf != nil {
			ev.Buf.Release()
		}
	case transport.OpClose:
		_ = u.Close()
		u.ch.Pipeline().FireChannelInactive()
	}
}

func (u *Unsafe) readReady() {
	if u.rw == nil {
		return
	}
	for !u.closed {
		buf, err := u.ch.Allocator().Acquire(u.readBufferSize)
		if err != nil {
			u.fail(err)
			return
		}
		view := buf.WritableBytesView()
		n, again, err := u.rw.Read(u.fd, view)
		if n > 0 {
			if advErr := buf.AdvanceWriter(n); advErr != nil {
				buf.Release()
				u.fail(advErr)
				return
			}
			u.ch.Pipeline().FireChannelRead(buf)
		} else {
			buf.Release()
		}
		if err != nil {
			u.fail(err)
			return
		}
		if again || n == 0 {
			return
		}
	}
}

func (u *Unsafe) writeReady(buf buffer.ByteBuf) error {
	defer buf.Release()
	if u.rw == nil {
		return ErrNoOutboundSink
	}
	for buf.ReadableBytes() > 0 {
		n, again, err := u.rw.Write(u.fd, buf.Bytes())
		if n > 0 {
			if skipErr := buf.SkipBytes(n); skipErr != nil {
				return skipErr
			}
		}
		if err != nil {
			return err
		}
		if again || n == 0 {
			return nil
		}
	}
	return nil
}

func (u *Unsafe) submitRead() error {
	buf, err := u.ch.Allocator().Acquire(u.readBufferSize)
	if err != nil {
		return err
	}
	err = u.poller.Submit(transport.IORequest{
		Op:        transport.OpRead,
		FD:        u.fd,
		ChannelID: u.ID(),
		Buf:       buf,
	})
	buf.Release()
	return err
}

func (u *Unsafe) fail(err error) {
	u.ch.Pipeline().FireExceptionCaught(err)
	_ = u.Close()
	u.ch.Pipeline().FireChannelInactive()
}

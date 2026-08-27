package channel

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

func (u *Unsafe) BeginRead() error {
	if !u.AutoRead() {
		return nil
	}
	return u.Read()
}

func (u *Unsafe) Read() error {
	if u.closed.Load() {
		return ErrPromiseFailed
	}
	if u.poller == nil || u.poller.Model() != transport.PollerCompletion {
		u.readReady()
		return nil
	}
	return u.submitRead()
}

func (u *Unsafe) AutoRead() bool {
	return OptionAutoRead.Get(u.ch.Options())
}

func (u *Unsafe) InitialInterest() transport.ReadyMask {
	return u.readInterest()
}

func (u *Unsafe) readReady() {
	if u.rw == nil {
		return
	}
	read := false
	messages := 0
	maxMessages := u.maxMessagesPerRead()
	for !u.closed.Load() && messages < maxMessages {
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
			read = true
			messages++
		} else {
			buf.Release()
		}
		if err != nil {
			u.fail(err)
			return
		}
		if n == 0 && !again {
			if read {
				u.ch.Pipeline().FireChannelReadComplete()
			}
			_ = u.Close()
			return
		}
		if again {
			if read {
				u.ch.Pipeline().FireChannelReadComplete()
			}
			return
		}
	}
	if read {
		u.ch.Pipeline().FireChannelReadComplete()
	}
}

func (u *Unsafe) submitRead() error {
	if u.readPending {
		return nil
	}
	buf, err := u.ch.Allocator().Acquire(u.readBufferSize)
	if err != nil {
		return err
	}
	req := u.prepareIORequest(transport.IORequest{
		Op:                      transport.OpRead,
		FD:                      u.fd,
		ChannelID:               u.ID(),
		Buf:                     buf,
		TransferBufferOwnership: true,
	})
	u.readPending = true
	err = u.poller.Submit(req)
	if err != nil {
		u.readPending = false
		buf.Release()
	}
	return err
}

func (u *Unsafe) readInterest() transport.ReadyMask {
	if u.AutoRead() {
		return transport.ReadyRead
	}
	return 0
}

func (u *Unsafe) maxMessagesPerRead() int {
	maxMessages := OptionMaxMessagesPerRead.Get(u.ch.Options())
	if maxMessages <= 0 {
		return 1
	}
	return maxMessages
}

func (u *Unsafe) prepareIORequest(req transport.IORequest) transport.IORequest {
	if !u.fixedBuffers || u.poller == nil || u.poller.Backend() != transport.BackendIOUring || req.Buf == nil {
		return req
	}
	idx, ok := buffer.FixedBufferIndex(req.Buf)
	if !ok {
		return req
	}
	req.UseFixedBuffer = true
	req.FixedBufferIndex = idx
	return req
}

package udp

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

type endpoint struct {
	id transport.ChannelID
	fd transport.FDRef

	loop  *transport.EventLoop
	ch    *channel.LocalChannel
	alloc buffer.Allocator

	readBufferSize int
	writeInterest  bool
	readPending    bool
	writePending   bool
	closed         bool
	inactiveFired  bool

	outHead *outboundDatagram
	outTail *outboundDatagram
	outFree *outboundDatagram

	server *Server
}

type outboundDatagram struct {
	datagram Datagram
	next     *outboundDatagram
}

func (e *endpoint) ID() transport.ChannelID {
	return e.id
}

func (e *endpoint) FD() transport.FDRef {
	return e.fd
}

func (e *endpoint) Channel() channel.Channel {
	return e.ch
}

func (e *endpoint) MarkRegistered() {
	if e.ch != nil {
		e.ch.Pipeline().FireChannelRegistered()
	}
}

func (e *endpoint) MarkDeregistered() {
	if e.ch != nil {
		e.ch.Pipeline().FireChannelUnregistered()
	}
}

func (e *endpoint) HandleEvent(ev transport.PollEvent) {
	if e.closed {
		if ev.Model == transport.PollerCompletion {
			releasePollEvent(ev)
		}
		return
	}
	if ev.Model == transport.PollerCompletion {
		e.handleCompletion(ev)
		return
	}
	if ev.Err != nil {
		e.fireException(ev.Err)
		_ = e.Close()
		return
	}
	if ev.Ready&(transport.ReadyError|transport.ReadyHangup) != 0 {
		_ = e.Close()
		return
	}
	if ev.Ready&transport.ReadyRead != 0 {
		e.readReady()
	}
	if ev.Ready&transport.ReadyWrite != 0 {
		if err := e.flushOutbound(); err != nil {
			e.fireException(err)
			_ = e.Close()
		}
	}
}

func (e *endpoint) Write(msg any) error {
	datagram, ok := asDatagram(msg)
	if !ok || !datagram.Valid() {
		releaseMessage(msg)
		return ErrInvalidDatagram
	}
	if e.closed {
		datagram.Release()
		return ErrServerClosed
	}
	if e.loop != nil && e.loop.Poller().Model() == transport.PollerCompletion {
		e.enqueue(datagram)
		return e.submitWriteCompletion()
	}
	if e.outHead != nil {
		e.enqueue(datagram)
		return e.enableWriteInterest()
	}
	again, err := sendDatagram(e.fd, datagram)
	if err != nil {
		datagram.Release()
		return err
	}
	if again {
		e.enqueue(datagram)
		return e.enableWriteInterest()
	}
	datagram.Release()
	return nil
}

func (e *endpoint) Flush() error {
	if e.closed {
		return ErrServerClosed
	}
	if e.loop != nil && e.loop.Poller().Model() == transport.PollerCompletion {
		return e.submitWriteCompletion()
	}
	return e.flushOutbound()
}

func (e *endpoint) IsWritable() bool {
	return !e.closed && e.outHead == nil && !e.writePending
}

func (e *endpoint) Close() error {
	if e.closed {
		return nil
	}
	e.closed = true
	e.releaseOutbound()
	err := closeFD(e.fd)
	if e.alloc != nil {
		if allocErr := e.alloc.Close(); err == nil {
			err = allocErr
		}
	}
	e.fireInactiveOnce()
	return err
}

func (e *endpoint) readReady() {
	if e.alloc == nil || e.ch == nil {
		return
	}
	read := false
	for !e.closed {
		buf, err := e.alloc.Acquire(e.readBufferSize)
		if err != nil {
			e.fireException(err)
			_ = e.Close()
			return
		}
		n, addr, again, err := recvDatagram(e.fd, buf.WritableBytesView())
		if again {
			buf.Release()
			break
		}
		if err != nil {
			buf.Release()
			e.fireException(err)
			_ = e.Close()
			return
		}
		if n > 0 {
			if err := buf.AdvanceWriter(n); err != nil {
				buf.Release()
				e.fireException(err)
				_ = e.Close()
				return
			}
		}
		if addr.IP == nil {
			buf.Release()
			break
		}
		e.ch.Pipeline().FireChannelRead(Datagram{Payload: buf, Addr: addr})
		read = true
	}
	if read {
		e.ch.Pipeline().FireChannelReadComplete()
	}
}

func (e *endpoint) handleCompletion(ev transport.PollEvent) {
	switch ev.Op {
	case transport.OpRead:
		e.handleReadCompletion(ev)
	case transport.OpWrite:
		e.handleWriteCompletion(ev)
	case transport.OpClose:
		_ = e.Close()
	}
}

func (e *endpoint) handleReadCompletion(ev transport.PollEvent) {
	e.readPending = false
	if ev.Err != nil {
		if ev.Buf != nil {
			ev.Buf.Release()
		}
		e.fireException(ev.Err)
		_ = e.Close()
		return
	}
	read := false
	if ev.N > 0 && ev.Buf != nil && ev.Addr.Valid() {
		e.ch.Pipeline().FireChannelRead(Datagram{Payload: ev.Buf, Addr: socketAddressToAddress(ev.Addr)})
		read = true
	} else if ev.Buf != nil {
		ev.Buf.Release()
	}
	if read {
		e.ch.Pipeline().FireChannelReadComplete()
	}
	if !e.closed {
		if err := e.submitReadCompletion(); err != nil {
			e.fireException(err)
			_ = e.Close()
		}
	}
}

func (e *endpoint) handleWriteCompletion(ev transport.PollEvent) {
	e.writePending = false
	if ev.Buf != nil {
		ev.Buf.Release()
	}
	if ev.Err != nil {
		e.fireException(ev.Err)
		_ = e.Close()
		return
	}
	e.dequeue()
	if e.outHead != nil {
		if err := e.submitWriteCompletion(); err != nil {
			e.fireException(err)
			_ = e.Close()
		}
		return
	}
	if e.ch != nil {
		e.ch.Pipeline().FireFlushComplete()
	}
}

func (e *endpoint) submitReadCompletion() error {
	if e.closed || e.readPending || e.alloc == nil {
		return nil
	}
	buf, err := e.alloc.Acquire(e.readBufferSize)
	if err != nil {
		return err
	}
	req := transport.IORequest{
		Op:        transport.OpRead,
		FD:        e.fd,
		ChannelID: e.id,
		Buf:       buf,
		Datagram:  true,
	}
	e.readPending = true
	err = e.loop.Poller().Submit(req)
	buf.Release()
	if err != nil {
		e.readPending = false
	}
	return err
}

func (e *endpoint) submitWriteCompletion() error {
	if e.closed || e.writePending || e.outHead == nil {
		return nil
	}
	addr, err := addressToSocketAddress(e.outHead.datagram.Addr)
	if err != nil {
		return err
	}
	e.writePending = true
	err = e.loop.Poller().Submit(transport.IORequest{
		Op:        transport.OpWrite,
		FD:        e.fd,
		ChannelID: e.id,
		Buf:       e.outHead.datagram.Payload,
		Addr:      addr,
		Datagram:  true,
	})
	if err != nil {
		e.writePending = false
	}
	return err
}

func (e *endpoint) flushOutbound() error {
	for e.outHead != nil {
		datagram := e.outHead.datagram
		again, err := sendDatagram(e.fd, datagram)
		if err != nil {
			return err
		}
		if again {
			return e.enableWriteInterest()
		}
		e.dequeue()
	}
	if err := e.disableWriteInterest(); err != nil {
		return err
	}
	if e.ch != nil {
		e.ch.Pipeline().FireFlushComplete()
	}
	return nil
}

func (e *endpoint) enqueue(datagram Datagram) {
	entry := e.acquireEntry(datagram)
	if e.outTail == nil {
		e.outHead = entry
		e.outTail = entry
		return
	}
	e.outTail.next = entry
	e.outTail = entry
}

func (e *endpoint) dequeue() {
	entry := e.outHead
	if entry == nil {
		return
	}
	e.outHead = entry.next
	if e.outHead == nil {
		e.outTail = nil
	}
	entry.datagram.Release()
	e.releaseEntry(entry)
}

func (e *endpoint) acquireEntry(datagram Datagram) *outboundDatagram {
	if e.outFree == nil {
		return &outboundDatagram{datagram: datagram}
	}
	entry := e.outFree
	e.outFree = entry.next
	entry.datagram = datagram
	entry.next = nil
	return entry
}

func (e *endpoint) releaseEntry(entry *outboundDatagram) {
	entry.datagram = Datagram{}
	entry.next = e.outFree
	e.outFree = entry
}

func (e *endpoint) releaseOutbound() {
	for e.outHead != nil {
		e.dequeue()
	}
	for e.outFree != nil {
		entry := e.outFree
		e.outFree = entry.next
		entry.next = nil
	}
}

func (e *endpoint) enableWriteInterest() error {
	if e.loop == nil || e.writeInterest {
		return nil
	}
	e.writeInterest = true
	return e.loop.Poller().Modify(e.fd, transport.ReadyRead|transport.ReadyWrite)
}

func (e *endpoint) disableWriteInterest() error {
	if e.loop == nil || !e.writeInterest {
		return nil
	}
	e.writeInterest = false
	return ignoreClosed(e.loop.Poller().Modify(e.fd, transport.ReadyRead))
}

func (e *endpoint) fireException(err error) {
	if e.ch != nil {
		e.ch.Pipeline().FireExceptionCaught(err)
	}
}

func (e *endpoint) fireInactiveOnce() {
	if e.inactiveFired {
		return
	}
	e.inactiveFired = true
	if e.ch != nil {
		e.ch.Pipeline().FireChannelInactive()
	}
}

func asDatagram(msg any) (Datagram, bool) {
	switch v := msg.(type) {
	case Datagram:
		return v, true
	case *Datagram:
		if v == nil {
			return Datagram{}, false
		}
		return *v, true
	default:
		return Datagram{}, false
	}
}

func releaseMessage(msg any) {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		buf.Release()
		return
	}
	if releasable, ok := msg.(interface{ Release() }); ok {
		releasable.Release()
	}
}

func releasePollEvent(ev transport.PollEvent) {
	if ev.Buf != nil {
		ev.Buf.Release()
	}
	for _, buf := range ev.Bufs {
		buf.Release()
	}
}

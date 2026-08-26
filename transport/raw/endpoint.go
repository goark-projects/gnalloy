package raw

import (
	"sync/atomic"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

type endpoint struct {
	id transport.ChannelID
	fd transport.FDRef

	loop   *transport.EventLoop
	ch     *channel.LocalChannel
	alloc  buffer.Allocator
	remote Address

	protocol       int
	readBufferSize int
	writeInterest  bool
	readPending    bool
	writePending   bool
	closed         atomic.Bool
	inactiveFired  atomic.Bool
	outboundBytes  atomic.Int64

	outHead *outboundPacket
	outTail *outboundPacket
	outFree *outboundPacket

	writeHighWatermark int64
	writeLowWatermark  int64
	writable           atomic.Bool

	server *Server
}

type outboundPacket struct {
	packet Packet
	next   *outboundPacket
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

func (e *endpoint) initBackpressure(w transport.WriteBufferWatermark) {
	w = transport.NormalizeWriteBufferWatermark(w)
	e.writeHighWatermark = int64(w.High)
	e.writeLowWatermark = int64(w.Low)
	e.writable.Store(true)
}

func (e *endpoint) HandleEvent(ev transport.PollEvent) {
	if e.closed.Load() {
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
		if e.AutoRead() {
			e.readReady()
		}
	}
	if ev.Ready&transport.ReadyWrite != 0 {
		if err := e.flushOutbound(); err != nil {
			e.fireException(err)
			_ = e.Close()
		}
	}
}

func (e *endpoint) Write(msg any) error {
	packet, ok := e.packetFromMessage(msg)
	if !ok {
		releaseMessage(msg)
		return ErrInvalidPacket
	}
	if packet.Protocol == 0 {
		packet.Protocol = e.protocol
	}
	if !packet.Valid() {
		packet.Release()
		return ErrInvalidPacket
	}
	if e.closed.Load() {
		packet.Release()
		return ErrServerClosed
	}
	if e.loop != nil && e.loop.Poller().Model() == transport.PollerCompletion {
		e.enqueue(packet)
		return e.submitWriteCompletion()
	}
	if e.outHead != nil {
		e.enqueue(packet)
		return e.enableWriteInterest()
	}
	again, err := sendPacket(e.fd, packet)
	if err != nil {
		packet.Release()
		return err
	}
	if again {
		e.enqueue(packet)
		return e.enableWriteInterest()
	}
	packet.Release()
	return nil
}

func (e *endpoint) Flush() error {
	if e.closed.Load() {
		return ErrServerClosed
	}
	if e.loop != nil && e.loop.Poller().Model() == transport.PollerCompletion {
		return e.submitWriteCompletion()
	}
	return e.flushOutbound()
}

func (e *endpoint) Read() error {
	if e.closed.Load() {
		return ErrServerClosed
	}
	if e.loop != nil && e.loop.Poller().Model() == transport.PollerCompletion {
		return e.submitReadCompletion()
	}
	e.readReady()
	return nil
}

func (e *endpoint) AutoRead() bool {
	return e.ch == nil || channel.OptionAutoRead.Get(e.ch.Options())
}

func (e *endpoint) InitialInterest() transport.ReadyMask {
	if e.AutoRead() {
		return transport.ReadyRead
	}
	return 0
}

func (e *endpoint) IsWritable() bool {
	return e.writable.Load()
}

func (e *endpoint) PendingOutboundBytes() int64 {
	return e.outboundBytes.Load()
}

func (e *endpoint) WriteBufferWatermark() transport.WriteBufferWatermark {
	return transport.WriteBufferWatermark{Low: int(e.writeLowWatermark), High: int(e.writeHighWatermark)}
}

func (e *endpoint) Close() error {
	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}
	e.closeWritability()
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
	for !e.closed.Load() {
		buf, err := e.alloc.Acquire(e.readBufferSize)
		if err != nil {
			e.fireException(err)
			_ = e.Close()
			return
		}
		n, addr, again, err := recvPacket(e.fd, buf.WritableBytesView())
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
		e.ch.Pipeline().FireChannelRead(Packet{Payload: buf, Addr: addr, Protocol: e.protocol})
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
		e.ch.Pipeline().FireChannelRead(Packet{Payload: ev.Buf, Addr: socketAddressToAddress(ev.Addr), Protocol: e.protocol})
		read = true
	} else if ev.Buf != nil {
		ev.Buf.Release()
	}
	if read {
		e.ch.Pipeline().FireChannelReadComplete()
	}
	if !e.closed.Load() && e.AutoRead() {
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
	if e.closed.Load() || e.readPending || e.alloc == nil {
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
	if e.closed.Load() || e.writePending || e.outHead == nil {
		return nil
	}
	addr, err := addressToSocketAddress(e.outHead.packet.Addr)
	if err != nil {
		return err
	}
	e.writePending = true
	err = e.loop.Poller().Submit(transport.IORequest{
		Op:        transport.OpWrite,
		FD:        e.fd,
		ChannelID: e.id,
		Buf:       e.outHead.packet.Payload,
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
		packet := e.outHead.packet
		again, err := sendPacket(e.fd, packet)
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

func (e *endpoint) enqueue(packet Packet) {
	entry := e.acquireEntry(packet)
	if e.outTail == nil {
		e.outHead = entry
		e.outTail = entry
		e.outboundBytes.Add(int64(packetSize(packet)))
		e.updateWritability()
		return
	}
	e.outTail.next = entry
	e.outTail = entry
	e.outboundBytes.Add(int64(packetSize(packet)))
	e.updateWritability()
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
	if pending := e.outboundBytes.Add(-int64(packetSize(entry.packet))); pending < 0 {
		e.outboundBytes.Store(0)
	}
	entry.packet.Release()
	e.releaseEntry(entry)
	e.updateWritability()
}

func (e *endpoint) acquireEntry(packet Packet) *outboundPacket {
	if e.outFree == nil {
		return &outboundPacket{packet: packet}
	}
	entry := e.outFree
	e.outFree = entry.next
	entry.packet = packet
	entry.next = nil
	return entry
}

func (e *endpoint) releaseEntry(entry *outboundPacket) {
	entry.packet = Packet{}
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

func (e *endpoint) updateWritability() {
	if e.closed.Load() {
		e.closeWritability()
		return
	}
	pending := e.outboundBytes.Load()
	if e.writable.Load() && pending >= e.writeHighWatermark {
		e.writable.Store(false)
		e.fireWritabilityChanged()
		return
	}
	if !e.writable.Load() && pending <= e.writeLowWatermark {
		e.writable.Store(true)
		e.fireWritabilityChanged()
	}
}

func (e *endpoint) closeWritability() {
	if e.writable.CompareAndSwap(true, false) {
		e.fireWritabilityChanged()
	}
}

func (e *endpoint) fireWritabilityChanged() {
	if e.ch != nil {
		e.ch.Pipeline().FireChannelWritabilityChanged()
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
	if !e.inactiveFired.CompareAndSwap(false, true) {
		return
	}
	if e.ch != nil {
		e.ch.Pipeline().FireChannelInactive()
	}
}

func asPacket(msg any) (Packet, bool) {
	switch v := msg.(type) {
	case Packet:
		return v, true
	case *Packet:
		if v == nil {
			return Packet{}, false
		}
		return *v, true
	default:
		return Packet{}, false
	}
}

func (e *endpoint) packetFromMessage(msg any) (Packet, bool) {
	if packet, ok := asPacket(msg); ok {
		return packet, true
	}
	buf, ok := msg.(buffer.ByteBuf)
	if !ok || buf == nil || !e.remote.Valid() {
		return Packet{}, false
	}
	return Packet{Payload: buf, Addr: e.remote, Protocol: e.protocol}, true
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

func packetSize(packet Packet) int {
	if packet.Payload == nil {
		return 0
	}
	return packet.Payload.ReadableBytes()
}

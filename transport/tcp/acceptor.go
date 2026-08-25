package tcp

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

type acceptor struct {
	id     transport.ChannelID
	server *Server
	fd     transport.FDRef
	family int
	loop   *transport.EventLoop
}

func (a *acceptor) ID() transport.ChannelID {
	return a.id
}

func (a *acceptor) FD() transport.FDRef {
	return a.fd
}

func (a *acceptor) HandleEvent(ev transport.PollEvent) {
	if a.server.closed.Load() {
		if ev.AcceptedFD.Valid() {
			_ = closeFD(ev.AcceptedFD)
		}
		return
	}
	if ev.Err != nil {
		if ev.AcceptedFD.Valid() {
			_ = closeFD(ev.AcceptedFD)
		}
		return
	}
	if ev.Model == transport.PollerCompletion {
		a.handleCompletion(ev)
		return
	}
	if ev.Ready&(transport.ReadyError|transport.ReadyHangup) != 0 {
		_ = a.Close()
		return
	}
	if ev.Ready&transport.ReadyRead != 0 {
		a.acceptReady()
	}
}

func (a *acceptor) Close() error {
	return closeFD(a.fd)
}

func (a *acceptor) acceptReady() {
	for !a.server.closed.Load() {
		fd, again, err := acceptTCP(a.fd)
		if err != nil || again {
			return
		}
		a.acceptChild(fd)
	}
}

func (a *acceptor) handleCompletion(ev transport.PollEvent) {
	if ev.Op != transport.OpAccept {
		return
	}
	if ev.Err == nil && ev.AcceptedFD.Valid() {
		if err := completeAccepted(a.fd, ev.AcceptedFD); err == nil {
			a.acceptChild(ev.AcceptedFD)
		} else {
			_ = closeFD(ev.AcceptedFD)
		}
	} else if ev.AcceptedFD.Valid() {
		_ = closeFD(ev.AcceptedFD)
	}
	if !ev.More && !a.server.closed.Load() && a.loop != nil {
		_ = a.submitAccept(a.loop)
	}
}

func (a *acceptor) acceptChild(fd transport.FDRef) {
	if err := setAcceptedOptions(fd, a.server.options); err != nil {
		_ = closeFD(fd)
		return
	}
	worker, err := a.server.workerGroup.Next()
	if err != nil {
		_ = closeFD(fd)
		return
	}
	alloc, err := a.server.allocatorFor(worker)
	if err != nil {
		_ = closeFD(fd)
		return
	}
	ch, unsafeCh := channel.NewUnsafeChannel(channel.UnsafeConfig{
		ID:             a.server.nextChannelID(),
		FD:             fd,
		Allocator:      alloc,
		Poller:         worker.Poller(),
		ReadWriter:     newNativeReadWriter(),
		CloseHook:      a.closeChildHook(worker),
		ReadBufferSize: a.server.options.readBufferSize,
		FixedBuffers:   a.server.options.iouringFixed,
		Timer:          worker.Timer(),
	})
	if err := a.server.childInitializer(ch); err != nil {
		_ = unsafeCh.Close()
		ch.Pipeline().FireExceptionCaught(err)
		return
	}
	if err := worker.Submit(func() {
		if a.server.closed.Load() {
			_ = unsafeCh.Close()
			return
		}
		if err := worker.Register(unsafeCh, transport.ReadyRead); err != nil {
			ch.Pipeline().FireExceptionCaught(err)
			_ = unsafeCh.Close()
			return
		}
		a.server.registerChild(worker, unsafeCh)
		_ = unsafeCh.Activate()
	}); err != nil {
		_ = unsafeCh.Close()
		ch.Pipeline().FireExceptionCaught(err)
	}
}

func (a *acceptor) closeChildHook(worker *transport.EventLoop) func(*channel.Unsafe) {
	return func(ch *channel.Unsafe) {
		a.server.unregisterChild(ch.ID())
		if worker != nil {
			_ = worker.Deregister(ch.ID())
		}
	}
}

func (a *acceptor) submitAccept(loop *transport.EventLoop) error {
	req, err := prepareAcceptRequest(transport.IORequest{
		Op:        transport.OpAccept,
		FD:        a.fd,
		ChannelID: a.id,
	}, a.family)
	if err != nil {
		return err
	}
	if err := loop.Poller().Submit(req); err != nil {
		if req.AcceptedFD.Valid() {
			_ = closeFD(req.AcceptedFD)
		}
		return err
	}
	return nil
}

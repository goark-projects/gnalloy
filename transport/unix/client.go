package unix

import (
	"context"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

// Dial 建立客户端 Unix domain socket Channel，并交给目标 EventLoop 管理。
func (t *Transport) Dial(ctx context.Context, cfg bootstrap.ClientConfig) (channel.Channel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.Group == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if cfg.Initializer == nil {
		cfg.Initializer = func(channel.Channel) error { return nil }
	}
	opts := t.cfg.socketOptions().withClientOptions(cfg.Options)
	fd, err := dialUnix(cfg.Address, opts)
	if err != nil {
		return nil, err
	}
	loop, err := cfg.Group.Next()
	if err != nil {
		_ = closeFD(fd)
		return nil, err
	}
	if opts.iouringFixed {
		_ = closeFD(fd)
		return nil, ErrUnsupportedFixedBuffers
	}
	alloc, err := t.clientAllocator(loop)
	if err != nil {
		_ = closeFD(fd)
		return nil, err
	}
	unsafeCfg := channel.UnsafeConfig{
		ID:                   t.nextChannelID(),
		FD:                   fd,
		Allocator:            alloc,
		Poller:               loop.Poller(),
		ReadWriter:           newNativeReadWriter(),
		ReadBufferSize:       opts.readBufferSize,
		WriteBufferWatermark: opts.writeBufferWatermark,
		Timer:                loop.Timer(),
	}
	var unsafeCh *channel.Unsafe
	unsafeCfg.CloseHook = func(ch *channel.Unsafe) {
		_ = loop.Deregister(ch.ID())
		_ = alloc.Close()
	}
	ch, unsafeCh, err := cfg.NewChannel(unsafeCfg)
	if err != nil {
		_ = closeFD(fd)
		_ = alloc.Close()
		return nil, err
	}
	cfg.Apply(ch)
	if err := cfg.Initializer(ch); err != nil {
		_ = unsafeCh.Close()
		ch.Pipeline().FireExceptionCaught(err)
		return nil, err
	}
	if err := loop.Invoke(ctx, func() error {
		if err := loop.Register(unsafeCh, unsafeCh.InitialInterest()); err != nil {
			return err
		}
		return unsafeCh.Activate()
	}); err != nil {
		_ = unsafeCh.Close()
		ch.Pipeline().FireExceptionCaught(err)
		return nil, err
	}
	return ch, nil
}

func (t *Transport) clientAllocator(loop *transport.EventLoop) (buffer.Allocator, error) {
	if t.cfg.AllocatorFactory != nil {
		return t.cfg.AllocatorFactory(loop)
	}
	return buffer.NewHeapAllocator(), nil
}

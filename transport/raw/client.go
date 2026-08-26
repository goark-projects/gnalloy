package raw

import (
	"context"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

// Dial 创建连接到默认远端 IP 的 raw Channel。
//
// 出站消息可以是 Packet，也可以是裸 ByteBuf；裸 ByteBuf 会使用配置的 IP 协议号
// 发往 Dial 地址。raw socket 权限和系统策略仍由操作系统决定。
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
	opts, err := t.cfg.socketOptions()
	if err != nil {
		return nil, err
	}
	remoteParsed, err := parseAddress(cfg.Address, opts.family)
	if err != nil {
		return nil, err
	}
	sock, err := listenRaw(clientBindAddress(opts.family), opts)
	if err != nil {
		return nil, err
	}
	loop, err := cfg.Group.Next()
	if err != nil {
		_ = closeFD(sock.fd)
		return nil, err
	}
	ep := &endpoint{
		id:             transport.ChannelID(t.nextID.Add(1)),
		fd:             sock.fd,
		protocol:       sock.protocol,
		readBufferSize: opts.readBufferSize,
		remote:         remoteParsed.Address(),
	}
	ep.initBackpressure(opts.writeBufferWatermark)
	ep.loop = loop
	err = loop.Invoke(ctx, func() error {
		alloc, err := t.clientAllocator(loop)
		if err != nil {
			return err
		}
		ep.alloc = alloc
		ep.ch = channel.NewLocalChannelWithTimer(ep.id, alloc, ep, loop.Timer())
		channel.OptionReadBufferSize.Set(ep.ch.Options(), ep.readBufferSize)
		channel.OptionWriteBufferWatermark.Set(ep.ch.Options(), ep.WriteBufferWatermark())
		cfg.Apply(ep.ch)
		if readBufferSize := channel.OptionReadBufferSize.Get(ep.ch.Options()); readBufferSize > 0 {
			ep.readBufferSize = readBufferSize
		}
		if err := cfg.Initializer(ep.ch); err != nil {
			return err
		}
		if err := loop.Register(ep, ep.InitialInterest()); err != nil {
			return err
		}
		ep.ch.Pipeline().FireChannelActive()
		if loop.Poller().Model() == transport.PollerCompletion && ep.AutoRead() {
			return ep.submitReadCompletion()
		}
		return nil
	})
	if err != nil {
		_ = ep.Close()
		return nil, err
	}
	return ep.ch, nil
}

func (t *Transport) clientAllocator(loop *transport.EventLoop) (buffer.Allocator, error) {
	if t.cfg.AllocatorFactory != nil {
		return t.cfg.AllocatorFactory(loop)
	}
	return buffer.NewHeapAllocator(), nil
}

func clientBindAddress(family Family) string {
	if family == FamilyIPv6 {
		return "::"
	}
	return "0.0.0.0"
}

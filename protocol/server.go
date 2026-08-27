package protocol

import (
	"context"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

// Server 提供和 ChannelExchanger 对称的应用协议服务端装配 API。
type Server struct {
	BossGroup   *transport.EventLoopGroup
	WorkerGroup *transport.EventLoopGroup
	Transport   bootstrap.ServerTransport
	Adapter     ServerAdapter
	Initializer bootstrap.ChildInitializer
	Handler     Handler
}

// Bind 使用背景上下文绑定服务端地址。
func (s Server) Bind(address string) (bootstrap.Server, error) {
	return s.BindContext(context.Background(), address)
}

// BindContext 绑定服务端地址，并把协议 handler 安装到每个 child Channel。
func (s Server) BindContext(ctx context.Context, address string) (bootstrap.Server, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.BossGroup == nil || s.WorkerGroup == nil || s.Transport == nil || s.Adapter == nil || s.Handler == nil {
		return nil, ErrInvalidConfig
	}
	return bootstrap.NewServerBootstrap().
		Group(s.BossGroup, s.WorkerGroup).
		Transport(s.Transport).
		ChildInitializer(func(ch channel.Channel) error {
			if s.Initializer != nil {
				if err := s.Initializer(ch); err != nil {
					return err
				}
			}
			return ch.Pipeline().AddLast("protocol-server", NewServerHandler(s.Adapter, s.Handler))
		}).
		BindContext(ctx, address)
}

// StreamServer 创建流式传输 request-response 服务端。
func StreamServer(bossGroup, workerGroup *transport.EventLoopGroup, serverTransport bootstrap.ServerTransport, handler Handler) Server {
	return Server{
		BossGroup:   bossGroup,
		WorkerGroup: workerGroup,
		Transport:   serverTransport,
		Adapter:     StreamAdapter{},
		Handler:     handler,
	}
}

// DatagramServer 创建 UDP datagram request-response 服务端。
func DatagramServer(bossGroup, workerGroup *transport.EventLoopGroup, serverTransport bootstrap.ServerTransport, handler Handler) Server {
	return Server{
		BossGroup:   bossGroup,
		WorkerGroup: workerGroup,
		Transport:   serverTransport,
		Adapter:     DatagramAdapter{},
		Handler:     handler,
	}
}

// PacketServer 创建 raw packet request-response 服务端。
func PacketServer(bossGroup, workerGroup *transport.EventLoopGroup, serverTransport bootstrap.ServerTransport, handler Handler) Server {
	return Server{
		BossGroup:   bossGroup,
		WorkerGroup: workerGroup,
		Transport:   serverTransport,
		Adapter:     PacketAdapter{},
		Handler:     handler,
	}
}

// FrameServer 创建 L2 frame request-response 服务端。
func FrameServer(bossGroup, workerGroup *transport.EventLoopGroup, serverTransport bootstrap.ServerTransport, handler Handler) Server {
	return Server{
		BossGroup:   bossGroup,
		WorkerGroup: workerGroup,
		Transport:   serverTransport,
		Adapter:     FrameAdapter{},
		Handler:     handler,
	}
}

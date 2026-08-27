package protocol

import (
	"context"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

// ChannelExchanger 通过 gnalloy Channel 执行一次 request-response 交换。
type ChannelExchanger struct {
	Group       *transport.EventLoopGroup
	Transport   bootstrap.ClientTransport
	Adapter     Adapter
	Initializer bootstrap.ChannelInitializer
	Timeout     time.Duration
}

// Exchange 打开一个客户端 Channel，写入请求并等待匹配响应。
func (e ChannelExchanger) Exchange(ctx context.Context, address string, payload []byte) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if e.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.Timeout)
		defer cancel()
	}
	if e.Group == nil || e.Transport == nil || e.Adapter == nil {
		return nil, ErrInvalidConfig
	}
	collector := newCollector(e.Adapter, payload)
	ch, err := bootstrap.NewDialer().
		Group(e.Group).
		Transport(e.Transport).
		Initializer(func(ch channel.Channel) error {
			if e.Initializer != nil {
				if err := e.Initializer(ch); err != nil {
					return err
				}
			}
			return ch.Pipeline().AddLast("protocol-response-collector", collector)
		}).
		DialContext(ctx, address)
	if err != nil {
		return nil, err
	}
	defer ch.Close()
	msg, err := e.Adapter.BuildRequest(ch, payload)
	if err != nil {
		return nil, err
	}
	if err := ch.WriteAndFlush(msg); err != nil {
		return nil, err
	}
	select {
	case response := <-collector.responses:
		return response.payload, response.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Stream 创建流式传输 request-response exchanger。
func Stream(group *transport.EventLoopGroup, transport bootstrap.ClientTransport) ChannelExchanger {
	return ChannelExchanger{Group: group, Transport: transport, Adapter: StreamAdapter{}}
}

// Datagram 创建 datagram 传输 request-response exchanger。
func Datagram(group *transport.EventLoopGroup, transport bootstrap.ClientTransport) ChannelExchanger {
	return ChannelExchanger{Group: group, Transport: transport, Adapter: DatagramAdapter{}}
}

// Packet 创建 raw packet 传输 request-response exchanger。
func Packet(group *transport.EventLoopGroup, transport bootstrap.ClientTransport) ChannelExchanger {
	return ChannelExchanger{Group: group, Transport: transport, Adapter: PacketAdapter{}}
}

// Frame 创建 L2 frame 传输 request-response exchanger。
func Frame(group *transport.EventLoopGroup, transport bootstrap.ClientTransport) ChannelExchanger {
	return ChannelExchanger{Group: group, Transport: transport, Adapter: FrameAdapter{}}
}

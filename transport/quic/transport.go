package quic

import (
	"context"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/udp"
)

const packetHandlerName = "quicPacketHandler"

// Transport 是 QUIC 最小包引擎的 ServerBootstrap 入口。
type Transport struct {
	cfg Config
}

func NewTransport(cfg Config) *Transport {
	return &Transport{cfg: cfg}
}

func (t *Transport) Bind(ctx context.Context, serverCfg bootstrap.ServerConfig) (bootstrap.Server, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if serverCfg.WorkerGroup == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if serverCfg.ChildInitializer == nil {
		return nil, bootstrap.ErrMissingChildHandler
	}
	qcfg, err := NormalizeConfig(t.cfg)
	if err != nil {
		return nil, err
	}
	initializer := serverCfg.ChildInitializer
	serverCfg.ChildInitializer = func(ch channel.Channel) error {
		handler := NewPacketHandler(PacketHandlerConfig{
			HeaderParseOptions: HeaderParseOptions{
				ShortDestinationIDLength: qcfg.ShortDestinationIDLength,
			},
			Router: NewConnectionIDRouter(qcfg.ActiveConnectionIDLimit),
		})
		if err := ch.Pipeline().AddLast(packetHandlerName, handler); err != nil {
			return err
		}
		return initializer(ch)
	}
	return udp.NewTransport(qcfg.UDP).Bind(ctx, serverCfg)
}

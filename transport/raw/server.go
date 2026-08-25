package raw

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

const closeControlTimeout = 5 * time.Second

type rawSocket struct {
	fd       transport.FDRef
	addr     string
	family   Family
	protocol int
}

// Transport 把 ServerBootstrap 接到 raw socket 生命周期。
type Transport struct {
	cfg    Config
	nextID atomic.Uint64
}

func NewTransport(cfg Config) *Transport {
	return &Transport{cfg: normalizeConfig(cfg)}
}

func (t *Transport) Bind(ctx context.Context, cfg bootstrap.ServerConfig) (bootstrap.Server, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.WorkerGroup == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if cfg.ChildInitializer == nil {
		return nil, bootstrap.ErrMissingChildHandler
	}
	opts, err := t.cfg.socketOptions()
	if err != nil {
		return nil, err
	}
	sock, err := listenRaw(cfg.Address, opts)
	if err != nil {
		return nil, err
	}
	server := &Server{
		addr:             sock.addr,
		childInitializer: cfg.ChildInitializer,
		allocatorFactory: t.cfg.AllocatorFactory,
		endpoints:        make([]*endpoint, 0, 1),
	}
	ep := &endpoint{
		id:             transport.ChannelID(t.nextID.Add(1)),
		fd:             sock.fd,
		protocol:       sock.protocol,
		readBufferSize: opts.readBufferSize,
		server:         server,
	}
	ep.initBackpressure(opts.writeBufferWatermark)
	server.endpoints = append(server.endpoints, ep)
	loop, err := cfg.WorkerGroup.Next()
	if err != nil {
		_ = server.Close()
		return nil, err
	}
	ep.loop = loop
	err = loop.Invoke(ctx, func() error {
		alloc, err := server.newAllocator(loop)
		if err != nil {
			return err
		}
		ep.alloc = alloc
		ep.ch = channel.NewLocalChannelWithTimer(ep.id, alloc, ep, loop.Timer())
		channel.OptionReadBufferSize.Set(ep.ch.Options(), ep.readBufferSize)
		channel.OptionWriteBufferWatermark.Set(ep.ch.Options(), ep.WriteBufferWatermark())
		if err := server.childInitializer(ep.ch); err != nil {
			return err
		}
		if err := loop.Register(ep, transport.ReadyRead); err != nil {
			return err
		}
		ep.ch.Pipeline().FireChannelActive()
		if loop.Poller().Model() == transport.PollerCompletion {
			return ep.submitReadCompletion()
		}
		return nil
	})
	if err != nil {
		_ = server.Close()
		return nil, err
	}
	return server, nil
}

type Server struct {
	addr string

	childInitializer bootstrap.ChildInitializer
	allocatorFactory AllocatorFactory

	endpoints []*endpoint
	closed    atomic.Bool
	once      sync.Once
	err       error
}

func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) EndpointCount() int {
	return len(s.endpoints)
}

func (s *Server) Close() error {
	s.once.Do(func() {
		s.closed.Store(true)
		var first error
		for _, ep := range s.endpoints {
			if ep == nil {
				continue
			}
			if ep.loop != nil {
				ctx, cancel := context.WithTimeout(context.Background(), closeControlTimeout)
				err := ep.loop.Invoke(ctx, func() error {
					_ = ep.loop.Deregister(ep.ID())
					return ep.Close()
				})
				cancel()
				if err != nil {
					_ = ep.Close()
					if first == nil {
						first = err
					}
				}
				continue
			}
			if err := ep.Close(); err != nil && first == nil {
				first = err
			}
		}
		s.err = first
	})
	return s.err
}

func (s *Server) newAllocator(loop *transport.EventLoop) (buffer.Allocator, error) {
	if s.closed.Load() {
		return nil, ErrServerClosed
	}
	if s.allocatorFactory != nil {
		return s.allocatorFactory(loop)
	}
	return buffer.NewHeapAllocator(), nil
}

func ignoreClosed(err error) error {
	if errors.Is(err, transport.ErrClosedPoller) {
		return nil
	}
	return err
}

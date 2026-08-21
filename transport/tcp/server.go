package tcp

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

const (
	closeControlTimeout = 5 * time.Second
	closeDrainTimeout   = 5 * time.Second
)

type listenSocket struct {
	fd     transport.FDRef
	addr   string
	family int
}

// Transport 把 ServerBootstrap 接到原生 TCP socket 生命周期。
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
	if cfg.BossGroup == nil || cfg.WorkerGroup == nil {
		return nil, bootstrap.ErrMissingGroup
	}
	if cfg.ChildInitializer == nil {
		return nil, bootstrap.ErrMissingChildHandler
	}
	opts := t.cfg.socketOptions()
	listeners, err := t.listenAll(cfg.Address, cfg.BossGroup.Size(), opts)
	if err != nil {
		return nil, err
	}

	server := &Server{
		addr:              listeners[0].addr,
		options:           opts,
		workerGroup:       cfg.WorkerGroup,
		childInitializer:  cfg.ChildInitializer,
		allocatorFactory:  t.cfg.AllocatorFactory,
		allocators:        make(map[transport.EventLoopID]buffer.Allocator, cfg.WorkerGroup.Size()),
		active:            make(map[transport.ChannelID]activeChild, 1024),
		transportIDSource: t,
		acceptors:         make([]*acceptor, 0, len(listeners)),
		bossLoops:         make([]*transport.EventLoop, 0, len(listeners)),
	}

	for _, ls := range listeners {
		a := &acceptor{id: t.nextChannelID(), server: server, fd: ls.fd, family: ls.family}
		server.acceptors = append(server.acceptors, a)
		bossLoop, err := cfg.BossGroup.RegisterNext(ctx, a, transport.ReadyRead, func(loop *transport.EventLoop, handler transport.EventHandler) error {
			a := handler.(*acceptor)
			a.loop = loop
			if loop.Poller().Model() == transport.PollerCompletion {
				return a.submitAccept(loop)
			}
			return nil
		})
		if err != nil {
			_ = server.Close()
			return nil, err
		}
		server.bossLoops = append(server.bossLoops, bossLoop)
	}
	return server, nil
}

func (t *Transport) nextChannelID() transport.ChannelID {
	return transport.ChannelID(t.nextID.Add(1))
}

func (t *Transport) listenAll(address string, bossSize int, opts socketOptions) ([]listenSocket, error) {
	count := 1
	if opts.reusePort && bossSize > 1 {
		if !reusePortSupported() {
			return nil, ErrUnsupportedReusePort
		}
		count = bossSize
	}
	listeners := make([]listenSocket, 0, count)
	for i := 0; i < count; i++ {
		bindAddress := address
		if i > 0 {
			bindAddress = listeners[0].addr
		}
		ls, err := listenTCP(bindAddress, opts)
		if err != nil {
			closeListenSockets(listeners)
			return nil, err
		}
		listeners = append(listeners, ls)
	}
	return listeners, nil
}

type Server struct {
	addr string

	options           socketOptions
	workerGroup       *transport.EventLoopGroup
	childInitializer  bootstrap.ChildInitializer
	allocatorFactory  AllocatorFactory
	transportIDSource *Transport

	bossLoops []*transport.EventLoop
	acceptors []*acceptor

	allocMu    sync.Mutex
	allocators map[transport.EventLoopID]buffer.Allocator

	activeMu sync.Mutex
	active   map[transport.ChannelID]activeChild

	closed atomic.Bool
	once   sync.Once
	err    error
}

type activeChild struct {
	loop *transport.EventLoop
	ch   *channel.Unsafe
}

func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) ListenerCount() int {
	return len(s.acceptors)
}

func (s *Server) Close() error {
	s.once.Do(func() {
		s.closed.Store(true)
		var first error
		for i, a := range s.acceptors {
			if a == nil {
				continue
			}
			if i < len(s.bossLoops) && s.bossLoops[i] != nil {
				loop := s.bossLoops[i]
				ctx, cancel := context.WithTimeout(context.Background(), closeControlTimeout)
				err := loop.Invoke(ctx, func() error {
					_ = loop.Deregister(a.ID())
					return a.Close()
				})
				cancel()
				if err != nil {
					_ = a.Close()
					if first == nil {
						first = err
					}
				}
				continue
			}
			if err := a.Close(); err != nil && first == nil {
				first = err
			}
		}
		if err := s.closeActiveChildren(); err != nil && first == nil {
			first = err
		}
		if err := s.closeAllocators(); err != nil && first == nil {
			first = err
		}
		s.err = first
	})
	return s.err
}

func (s *Server) nextChannelID() transport.ChannelID {
	return s.transportIDSource.nextChannelID()
}

func (s *Server) ActiveConnectionCount() int {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	return len(s.active)
}

func (s *Server) AllocatorStats() []buffer.AllocatorStats {
	s.allocMu.Lock()
	defer s.allocMu.Unlock()
	ids := make([]transport.EventLoopID, 0, len(s.allocators))
	for id := range s.allocators {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i int, j int) bool { return ids[i] < ids[j] })
	stats := make([]buffer.AllocatorStats, 0, len(ids))
	for _, id := range ids {
		alloc := s.allocators[id]
		if alloc == nil {
			continue
		}
		if observed, ok := alloc.(buffer.StatAllocator); ok {
			stats = append(stats, observed.Stats())
			continue
		}
		stats = append(stats, buffer.AllocatorStats{})
	}
	return stats
}

func (s *Server) registerChild(loop *transport.EventLoop, ch *channel.Unsafe) {
	if loop == nil || ch == nil {
		return
	}
	s.activeMu.Lock()
	if s.active == nil {
		s.active = make(map[transport.ChannelID]activeChild, 1024)
	}
	s.active[ch.ID()] = activeChild{loop: loop, ch: ch}
	s.activeMu.Unlock()
}

func (s *Server) unregisterChild(id transport.ChannelID) {
	s.activeMu.Lock()
	delete(s.active, id)
	s.activeMu.Unlock()
}

func (s *Server) closeActiveChildren() error {
	s.activeMu.Lock()
	children := make([]activeChild, 0, len(s.active))
	for _, child := range s.active {
		children = append(children, child)
	}
	s.activeMu.Unlock()

	var first error
	for _, child := range children {
		if child.ch == nil {
			continue
		}
		if child.loop != nil {
			ctx, cancel := context.WithTimeout(context.Background(), closeControlTimeout)
			err := child.loop.Invoke(ctx, func() error {
				return child.ch.Close()
			})
			cancel()
			if err != nil {
				_ = child.ch.Close()
				if first == nil {
					first = err
				}
			}
			if waitErr := s.waitChildInactive(child.ch.ID(), closeDrainTimeout); waitErr != nil && first == nil {
				first = waitErr
			}
			continue
		}
		if err := child.ch.Close(); err != nil && first == nil {
			first = err
		}
		if waitErr := s.waitChildInactive(child.ch.ID(), closeDrainTimeout); waitErr != nil && first == nil {
			first = waitErr
		}
	}
	return first
}

func (s *Server) waitChildInactive(id transport.ChannelID, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		s.activeMu.Lock()
		_, ok := s.active[id]
		s.activeMu.Unlock()
		if !ok {
			return nil
		}
		if time.Now().After(deadline) {
			s.unregisterChild(id)
			return ErrCloseActiveTimeout
		}
		time.Sleep(time.Millisecond)
	}
}

func (s *Server) allocatorFor(loop *transport.EventLoop) (buffer.Allocator, error) {
	if s.closed.Load() {
		return nil, ErrServerClosed
	}
	s.allocMu.Lock()
	defer s.allocMu.Unlock()
	if s.closed.Load() || s.allocators == nil {
		return nil, ErrServerClosed
	}
	if alloc := s.allocators[loop.ID()]; alloc != nil {
		return alloc, nil
	}
	var (
		alloc buffer.Allocator
		err   error
	)
	if s.allocatorFactory != nil {
		alloc, err = s.allocatorFactory(loop)
	} else {
		alloc = buffer.NewHeapAllocator()
	}
	if err != nil {
		return nil, err
	}
	s.allocators[loop.ID()] = alloc
	return alloc, nil
}

func (s *Server) closeAllocators() error {
	s.allocMu.Lock()
	allocators := make([]buffer.Allocator, 0, len(s.allocators))
	for id, alloc := range s.allocators {
		delete(s.allocators, id)
		if alloc != nil {
			allocators = append(allocators, alloc)
		}
	}
	s.allocators = nil
	s.allocMu.Unlock()

	var first error
	for _, alloc := range allocators {
		if err := alloc.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func closeListenSockets(listeners []listenSocket) {
	for _, ls := range listeners {
		_ = closeFD(ls.fd)
	}
}

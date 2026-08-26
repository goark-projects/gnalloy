package tcp

import (
	"context"
	"errors"
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
		serverConfig:      cfg,
		workerGroup:       cfg.WorkerGroup,
		childInitializer:  cfg.ChildInitializer,
		allocatorFactory:  t.cfg.AllocatorFactory,
		allocators:        make(map[transport.EventLoopID]allocatorState, cfg.WorkerGroup.Size()),
		active:            make(map[transport.ChannelID]activeChild, 1024),
		transportIDSource: t,
		acceptors:         make([]*acceptor, 0, len(listeners)),
		bossLoops:         make([]*transport.EventLoop, 0, len(listeners)),
	}

	if opts.iouringFixed {
		for _, worker := range cfg.WorkerGroup.Loops() {
			if _, err := server.allocatorFor(worker); err != nil {
				closeListenSockets(listeners)
				_ = server.closeAllocators()
				return nil, err
			}
		}
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
	serverConfig      bootstrap.ServerConfig
	workerGroup       *transport.EventLoopGroup
	childInitializer  bootstrap.ChildInitializer
	allocatorFactory  AllocatorFactory
	transportIDSource *Transport

	bossLoops []*transport.EventLoop
	acceptors []*acceptor

	allocMu    sync.Mutex
	allocators map[transport.EventLoopID]allocatorState

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

type allocatorState struct {
	loop         *transport.EventLoop
	alloc        buffer.Allocator
	fixedBuffers bool
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
		state := s.allocators[id]
		if state.alloc == nil {
			continue
		}
		if observed, ok := state.alloc.(buffer.StatAllocator); ok {
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
	if state := s.allocators[loop.ID()]; state.alloc != nil {
		return state.alloc, nil
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
	state := allocatorState{loop: loop, alloc: alloc}
	if s.options.iouringFixed {
		if err := s.registerFixedBuffers(loop, alloc); err != nil {
			_ = alloc.Close()
			return nil, err
		}
		state.fixedBuffers = true
	}
	s.allocators[loop.ID()] = state
	return alloc, nil
}

func (s *Server) registerFixedBuffers(loop *transport.EventLoop, alloc buffer.Allocator) error {
	if loop == nil || loop.Poller().Backend() != transport.BackendIOUring {
		return ErrUnsupportedFixedBuffers
	}
	provider, ok := alloc.(buffer.FixedBufferProvider)
	if !ok {
		return ErrUnsupportedFixedBuffers
	}
	buffers := provider.FixedBuffers()
	if len(buffers) == 0 {
		return ErrUnsupportedFixedBuffers
	}
	registrar, ok := loop.Poller().(transport.BufferRegistrar)
	if !ok {
		return ErrUnsupportedFixedBuffers
	}
	return loop.Invoke(context.Background(), func() error {
		return registrar.RegisterBuffers(buffers)
	})
}

func (s *Server) closeAllocators() error {
	s.allocMu.Lock()
	states := make([]allocatorState, 0, len(s.allocators))
	for id, state := range s.allocators {
		delete(s.allocators, id)
		if state.alloc != nil {
			states = append(states, state)
		}
	}
	s.allocators = nil
	s.allocMu.Unlock()

	var first error
	for _, state := range states {
		if err := s.unregisterFixedBuffers(state); err != nil && first == nil {
			first = err
			continue
		}
		if err := state.alloc.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (s *Server) unregisterFixedBuffers(state allocatorState) error {
	if !state.fixedBuffers || state.loop == nil {
		return nil
	}
	registrar, ok := state.loop.Poller().(transport.BufferRegistrar)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), closeControlTimeout)
	defer cancel()
	err := state.loop.Invoke(ctx, registrar.UnregisterBuffers)
	if errors.Is(err, transport.ErrClosedPoller) {
		return nil
	}
	return err
}

func closeListenSockets(listeners []listenSocket) {
	for _, ls := range listeners {
		_ = closeFD(ls.fd)
	}
}

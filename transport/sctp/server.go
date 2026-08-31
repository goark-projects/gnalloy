package sctp

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

type Server struct {
	addr string

	options           socketOptions
	serverConfig      bootstrap.ServerConfig
	workerGroup       *transport.EventLoopGroup
	childInitializer  bootstrap.ChildInitializer
	allocatorFactory  AllocatorFactory
	transportIDSource *Transport

	bossLoop *transport.EventLoop
	acceptor *acceptor

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

func (s *Server) Close() error {
	s.once.Do(func() {
		s.closed.Store(true)
		var first error
		if s.acceptor != nil && s.bossLoop != nil {
			ctx, cancel := context.WithTimeout(context.Background(), closeControlTimeout)
			err := s.bossLoop.Invoke(ctx, func() error {
				_ = s.bossLoop.Deregister(s.acceptor.ID())
				return s.acceptor.Close()
			})
			cancel()
			if err != nil {
				_ = s.acceptor.Close()
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
		stats = append(stats, transport.AllocatorStatsForEventLoop(id, s.allocators[id]))
	}
	return stats
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
	if state := s.allocators[loop.ID()]; state != nil {
		return state, nil
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

func (s *Server) registerChild(loop *transport.EventLoop, ch *channel.Unsafe) {
	if loop == nil || ch == nil {
		return
	}
	s.activeMu.Lock()
	if s.active == nil {
		s.active = make(map[transport.ChannelID]activeChild, 128)
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
			err := child.loop.Invoke(ctx, child.ch.Close)
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

func closeIfValid(fd transport.FDRef) {
	if fd.Valid() {
		_ = closeFD(fd)
	}
}

func ignoreClosed(err error) error {
	if errors.Is(err, transport.ErrClosedPoller) {
		return nil
	}
	return err
}

package local

import (
	"context"
	"sync"
	"sync/atomic"

	"goark.dev/gnalloy/bootstrap"
)

// Server 是 local Bind 后返回的服务端句柄。
type Server struct {
	address string
	cfg     Config
	boot    bootstrap.ServerConfig

	mu        sync.Mutex
	endpoints map[*endpoint]struct{}
	closed    atomic.Bool
}

func newServer(address string, cfg Config, boot bootstrap.ServerConfig) *Server {
	return &Server{
		address:   address,
		cfg:       cfg,
		boot:      boot,
		endpoints: make(map[*endpoint]struct{}),
	}
}

func (s *Server) Addr() string {
	if s == nil {
		return ""
	}
	return s.address
}

func (s *Server) Close() error {
	if s == nil || !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	unregisterServer(s.address, s)
	for _, ep := range s.snapshotEndpoints() {
		_ = ep.Close()
	}
	return nil
}

func (s *Server) accept(ctx context.Context) (*endpoint, error) {
	if s == nil || s.closed.Load() {
		return nil, ErrClosed
	}
	transport := &Transport{cfg: s.cfg}
	ep, err := transport.newEndpoint(ctx, s.boot.WorkerGroup, s.cfg, s.boot.ApplyChild, s.boot.ChildInitializer)
	if err != nil {
		return nil, err
	}
	ep.server = s
	s.mu.Lock()
	if s.closed.Load() {
		s.mu.Unlock()
		_ = ep.Close()
		return nil, ErrClosed
	}
	s.endpoints[ep] = struct{}{}
	s.mu.Unlock()
	return ep, nil
}

func (s *Server) removeEndpoint(ep *endpoint) {
	if s == nil || ep == nil {
		return
	}
	s.mu.Lock()
	delete(s.endpoints, ep)
	s.mu.Unlock()
}

func (s *Server) snapshotEndpoints() []*endpoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*endpoint, 0, len(s.endpoints))
	for ep := range s.endpoints {
		out = append(out, ep)
	}
	s.endpoints = make(map[*endpoint]struct{})
	return out
}

func (s *Server) isClosed() bool {
	return s == nil || s.closed.Load()
}

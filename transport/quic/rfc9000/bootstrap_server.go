package rfc9000

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/transport"
)

const serverCloseTimeout = 5 * time.Second

type serverConfig struct {
	listener  Listener
	cfg       bootstrap.ServerConfig
	transport *Transport
	ctx       context.Context
	cancel    context.CancelFunc
}

// Server 是 QUIC Bootstrap 绑定后返回的服务端句柄。
type Server struct {
	addr     string
	listener Listener
	cfg      bootstrap.ServerConfig
	source   *Transport
	ctx      context.Context
	cancel   context.CancelFunc

	wg sync.WaitGroup

	mu      sync.Mutex
	conns   map[Connection]struct{}
	streams map[transport.ChannelID]*streamEndpoint

	closed atomic.Bool
	once   sync.Once
	err    error
}

func newServer(cfg serverConfig) *Server {
	return &Server{
		addr:     cfg.listener.Addr().String(),
		listener: cfg.listener,
		cfg:      cfg.cfg,
		source:   cfg.transport,
		ctx:      cfg.ctx,
		cancel:   cfg.cancel,
		conns:    make(map[Connection]struct{}, 16),
		streams:  make(map[transport.ChannelID]*streamEndpoint, 128),
	}
}

func (s *Server) start() {
	s.wg.Add(1)
	go s.acceptConnections()
}

// Addr 返回监听地址。
func (s *Server) Addr() string {
	if s == nil {
		return ""
	}
	return s.addr
}

// Close 停止接受新连接并关闭已接受的 stream channel。
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		s.closed.Store(true)
		if s.cancel != nil {
			s.cancel()
		}
		var first error
		if s.listener != nil {
			if err := s.listener.Close(); err != nil && !errors.Is(err, ErrClosed) {
				first = err
			}
		}
		for _, endpoint := range s.snapshotStreams() {
			if err := endpoint.Close(); err != nil && first == nil {
				first = err
			}
		}
		for _, conn := range s.snapshotConnections() {
			if err := conn.CloseWithError(0, "server closed"); err != nil && first == nil {
				first = err
			}
		}
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(serverCloseTimeout):
			if first == nil {
				first = context.DeadlineExceeded
			}
		}
		s.err = first
	})
	return s.err
}

func (s *Server) acceptConnections() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept(s.ctx)
		if err != nil {
			if s.isClosed() || errors.Is(err, context.Canceled) {
				return
			}
			return
		}
		s.registerConnection(conn)
		s.wg.Add(1)
		go s.acceptStreams(conn)
	}
}

func (s *Server) acceptStreams(conn Connection) {
	defer s.wg.Done()
	defer s.unregisterConnection(conn)
	for {
		stream, err := conn.AcceptStream(s.ctx)
		if err != nil {
			return
		}
		if err := s.initStream(stream); err != nil {
			stream.CancelRead(0)
			stream.CancelWrite(0)
		}
	}
}

func (s *Server) initStream(stream Stream) error {
	loop, err := s.cfg.WorkerGroup.Next()
	if err != nil {
		return err
	}
	endpoint, err := newStreamEndpoint(streamEndpointConfig{
		id:     s.source.nextChannelID(),
		stream: stream,
		timer:  loop.Timer(),
	})
	if err != nil {
		return err
	}
	endpoint.onClose = func() {
		s.unregisterStream(endpoint.ID())
	}
	s.cfg.ApplyChild(endpoint.Channel())
	endpoint.applyOptions()
	if err := s.cfg.ChildInitializer(endpoint.Channel()); err != nil {
		_ = endpoint.Close()
		return err
	}
	s.registerStream(endpoint)
	endpoint.activate(s.ctx)
	return nil
}

func (s *Server) registerConnection(conn Connection) {
	s.mu.Lock()
	s.conns[conn] = struct{}{}
	s.mu.Unlock()
}

func (s *Server) unregisterConnection(conn Connection) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
}

func (s *Server) registerStream(endpoint *streamEndpoint) {
	if endpoint == nil {
		return
	}
	s.mu.Lock()
	s.streams[endpoint.ID()] = endpoint
	s.mu.Unlock()
}

func (s *Server) unregisterStream(id transport.ChannelID) {
	s.mu.Lock()
	delete(s.streams, id)
	s.mu.Unlock()
}

func (s *Server) snapshotConnections() []Connection {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Connection, 0, len(s.conns))
	for conn := range s.conns {
		out = append(out, conn)
	}
	return out
}

func (s *Server) snapshotStreams() []*streamEndpoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*streamEndpoint, 0, len(s.streams))
	for _, endpoint := range s.streams {
		out = append(out, endpoint)
	}
	return out
}

func (s *Server) isClosed() bool {
	return s == nil || s.closed.Load()
}

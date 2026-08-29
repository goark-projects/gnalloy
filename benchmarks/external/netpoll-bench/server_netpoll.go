//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package main

import (
	"context"
	"sync"
	"time"

	"goark.dev/gnalloy/benchmarks/external/internal/benchhttp"

	"github.com/cloudwego/netpoll"
)

type echoServer struct {
	addr     string
	loop     netpoll.EventLoop
	listener netpoll.Listener
	errCh    chan error
}

func startEchoServer(ctx context.Context, cfg config) (*echoServer, error) {
	addr, err := resolveListenAddress(cfg.Addr)
	if err != nil {
		return nil, err
	}
	listener, err := netpoll.CreateListener("tcp", addr)
	if err != nil {
		return nil, err
	}
	response := netpollHTTPResponse(cfg)
	states := sync.Map{}
	loop, err := netpoll.NewEventLoop(func(_ context.Context, conn netpoll.Connection) error {
		if response != nil {
			return handleHTTP1(conn, response, &states)
		}
		reader := conn.Reader()
		for {
			n := reader.Len()
			if n == 0 {
				return nil
			}
			payload, err := reader.Next(n)
			if err != nil {
				return err
			}
			_, writeErr := conn.Write(payload)
			releaseErr := reader.Release()
			if writeErr != nil {
				return writeErr
			}
			if releaseErr != nil {
				return releaseErr
			}
		}
	})
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	server := &echoServer{addr: listener.Addr().String(), loop: loop, listener: listener, errCh: make(chan error, 1)}
	go func() {
		server.errCh <- loop.Serve(listener)
	}()

	select {
	case err := <-server.errCh:
		_ = listener.Close()
		return nil, err
	case <-time.After(20 * time.Millisecond):
		return server, nil
	case <-ctx.Done():
		_ = listener.Close()
		return nil, ctx.Err()
	}
}

type netpollHTTPState struct {
	once   sync.Once
	parser benchhttp.ServerState
}

func handleHTTP1(conn netpoll.Connection, response []byte, states *sync.Map) error {
	value, _ := states.LoadOrStore(conn, &netpollHTTPState{})
	state := value.(*netpollHTTPState)
	state.once.Do(func() {
		_ = conn.AddCloseCallback(func(connection netpoll.Connection) error {
			states.Delete(connection)
			return nil
		})
	})
	reader := conn.Reader()
	for {
		n := reader.Len()
		if n == 0 {
			return nil
		}
		payload, err := reader.Next(n)
		if err != nil {
			return err
		}
		count := state.parser.AppendAndCountRequests(payload)
		releaseErr := reader.Release()
		if releaseErr != nil {
			return releaseErr
		}
		for i := 0; i < count; i++ {
			if _, err := conn.Write(response); err != nil {
				return err
			}
		}
	}
}

func netpollHTTPResponse(cfg config) []byte {
	if cfg.Protocol != "http1" {
		return nil
	}
	return benchhttp.ResponseBytes(cfg.Payload)
}

func (s *echoServer) stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.loop.Shutdown(ctx)
	_ = s.listener.Close()
	select {
	case <-s.errCh:
	case <-ctx.Done():
	}
}

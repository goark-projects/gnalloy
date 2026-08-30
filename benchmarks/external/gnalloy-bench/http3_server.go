package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"

	"goark.dev/gnalloy/benchmarks/external/internal/benchh3"
	"goark.dev/gnalloy/channel"
	codechttp3 "goark.dev/gnalloy/codec/http3"
	h3transport "goark.dev/gnalloy/transport/http3"
	"goark.dev/gnalloy/transport/quic/rfc9000"
)

type http3Server struct {
	addr     string
	listener rfc9000.Listener
	body     []byte
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func startHTTP3Server(parent context.Context, cfg config) (*http3Server, error) {
	if parent == nil {
		parent = context.Background()
	}
	tlsConfig, err := serverTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	listener, err := rfc9000.ListenAddr(cfg.Addr, rfc9000.Config{
		TLS:        tlsConfig,
		NextProtos: []string{http3ALPN(cfg)},
	})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	server := &http3Server{
		addr:     listener.Addr().String(),
		listener: listener,
		body:     benchh3.ResponseBody(cfg.Payload),
		ctx:      ctx,
		cancel:   cancel,
	}
	server.wg.Add(1)
	go server.accept()
	return server, nil
}

func (s *http3Server) stop() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.wg.Wait()
}

func (s *http3Server) accept() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept(s.ctx)
		if err != nil {
			return
		}
		s.wg.Add(1)
		go s.serveConn(conn)
	}
}

func (s *http3Server) serveConn(conn rfc9000.Connection) {
	defer s.wg.Done()
	defer conn.CloseWithError(0, "benchmark done")
	session, err := h3transport.NewSession(conn, h3transport.Config{AllowedALPN: []string{http3ALPNValue}})
	if err != nil {
		return
	}
	for {
		streamCh, err := session.AcceptRequestStream(s.ctx)
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			_ = serveHTTP3Stream(s.ctx, streamCh, s.body)
		}()
	}
}

func serveHTTP3Stream(ctx context.Context, streamCh *h3transport.StreamChannel, body []byte) error {
	handler := &http3BenchmarkHandler{
		body: body,
		fields: []codechttp3.HeaderField{
			{Name: ":status", Value: "200"},
			{Name: "content-type", Value: "application/octet-stream"},
			{Name: "content-length", Value: strconv.Itoa(len(body))},
		},
	}
	if err := streamCh.Channel().Pipeline().AddLast("handler", handler); err != nil {
		return err
	}
	for !handler.responded {
		_, err := streamCh.ReadOnce(ctx)
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) && handler.responded {
			break
		}
		return err
	}
	return streamCh.Close()
}

type http3BenchmarkHandler struct {
	body      []byte
	fields    []codechttp3.HeaderField
	responded bool
}

func (h *http3BenchmarkHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if _, ok := msg.(codechttp3.HeadersBlock); !ok {
		ctx.FireChannelRead(msg)
		return
	}
	h.responded = true
	body, err := ctx.Channel().Allocator().Acquire(len(h.body))
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if _, err := body.WriteBytes(h.body); err != nil {
		body.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Write(codechttp3.HeadersBlock{Fields: h.fields}); err != nil {
		body.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Write(codechttp3.DataFrame{Data: body}); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Flush(); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func http3ALPN(cfg config) string {
	protocols := alpnProtocols(cfg.ALPN)
	if len(protocols) == 0 {
		return http3ALPNValue
	}
	return protocols[0]
}

func ensureHTTP3Config(cfg config) error {
	if cfg.TLSVersion != tlsVersion13 {
		return fmt.Errorf("%w: HTTP/3 requires TLS 1.3", errInvalidConfig)
	}
	if http3ALPN(cfg) != http3ALPNValue {
		return fmt.Errorf("%w: HTTP/3 requires ALPN h3", errInvalidConfig)
	}
	return nil
}

const http3ALPNValue = "h3"

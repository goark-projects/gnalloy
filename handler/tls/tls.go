package tls

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	cryptotls "crypto/tls"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

type Mode uint8

const (
	ModeClient Mode = iota + 1
	ModeServer
)

type Config struct {
	TLS *cryptotls.Config
	// StartTLS 表示连接建立后先透传明文，收到 StartEvent 后再启动 TLS 握手。
	StartTLS bool
	// VerifyPeerName 在握手完成后对对端证书执行主机名校验，空值表示不额外校验。
	VerifyPeerName string
}

type HandshakeEvent struct {
	ServerName         string
	NegotiatedProtocol string
	DidResume          bool
	CipherSuite        uint16
	Version            uint16
}

// StartEvent 触发 StartTLS 模式下的 TLS 握手。
type StartEvent struct{}

type Handler struct {
	mode Mode
	cfg  Config

	raw  *memoryConn
	conn *cryptotls.Conn

	startOnce sync.Once
	closeOnce sync.Once
	closed    chan struct{}
	ready     chan struct{}
	plain     chan []byte
	app       chan []byte
	events    chan HandshakeEvent
	errs      chan error

	handshake bool
	active    bool
	started   atomic.Bool
}

func Client(cfg Config) *Handler {
	return newHandler(ModeClient, cfg)
}

func Server(cfg Config) *Handler {
	return newHandler(ModeServer, cfg)
}

func newHandler(mode Mode, cfg Config) *Handler {
	return &Handler{
		mode:   mode,
		cfg:    cfg,
		closed: make(chan struct{}),
		ready:  make(chan struct{}),
		plain:  make(chan []byte, 32),
		app:    make(chan []byte, 32),
		events: make(chan HandshakeEvent, 4),
		errs:   make(chan error, 8),
	}
}

func (h *Handler) HandlerAdded(*channel.HandlerContext) error {
	if h.mode != ModeClient && h.mode != ModeServer {
		return ErrInvalidConfig
	}
	if !h.cfg.StartTLS {
		h.ensureStarted()
	}
	return nil
}

func (h *Handler) ChannelActive(ctx *channel.HandlerContext) {
	if h.cfg.StartTLS && !h.started.Load() {
		h.active = true
		ctx.FireChannelActive()
		return
	}
	h.ensureStarted()
	h.drain(ctx, true, h.mode == ModeClient)
}

func (h *Handler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if h.cfg.StartTLS && !h.started.Load() {
		ctx.FireChannelRead(msg)
		return
	}
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		if h.handshake {
			ctx.FireChannelRead(msg)
			return
		}
		release(msg)
		return
	}
	h.ensureStarted()
	err := h.raw.feed(buf.Bytes())
	buf.Release()
	if err != nil {
		h.fail(ctx, err)
		return
	}
	h.drain(ctx, true, true)
}

func (h *Handler) ChannelInactive(ctx *channel.HandlerContext) {
	h.close()
	ctx.FireChannelInactive()
}

func (h *Handler) Write(ctx *channel.HandlerContext, msg any) error {
	if h.cfg.StartTLS && !h.started.Load() {
		return ctx.Write(msg)
	}
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return ctx.Write(msg)
	}
	data := append([]byte(nil), buf.Bytes()...)
	buf.Release()
	if len(data) == 0 {
		return nil
	}
	h.ensureStarted()
	select {
	case h.app <- data:
	case <-h.closed:
		return io.ErrClosedPipe
	}
	h.drain(ctx, false, true)
	return nil
}

func (h *Handler) Flush(ctx *channel.HandlerContext) error {
	if h.cfg.StartTLS && !h.started.Load() {
		return ctx.Flush()
	}
	h.drain(ctx, false, true)
	return ctx.Flush()
}

func (h *Handler) UserEventTriggered(ctx *channel.HandlerContext, event any) {
	if _, ok := event.(StartEvent); ok {
		h.ensureStarted()
		h.drain(ctx, true, h.mode == ModeClient)
		return
	}
	ctx.FireUserEventTriggered(event)
}

func (h *Handler) Close(ctx *channel.HandlerContext) error {
	h.close()
	if !h.started.Load() {
		return ctx.Close()
	}
	h.drain(ctx, true, false)
	return ctx.Close()
}

func (h *Handler) ensureStarted() {
	h.startOnce.Do(func() {
		h.raw = newMemoryConn()
		cfg := &cryptotls.Config{}
		if h.cfg.TLS != nil {
			cfg = h.cfg.TLS.Clone()
		}
		if h.mode == ModeServer {
			h.conn = cryptotls.Server(h.raw, cfg)
		} else {
			h.conn = cryptotls.Client(h.raw, cfg)
		}
		h.started.Store(true)
		go h.runHandshake()
		go h.runWriter()
	})
}

func (h *Handler) runHandshake() {
	if err := h.conn.Handshake(); err != nil {
		h.sendErr(err)
		return
	}
	state := h.conn.ConnectionState()
	if h.cfg.VerifyPeerName != "" {
		if err := verifyPeerName(state, h.cfg.VerifyPeerName); err != nil {
			h.sendErr(err)
			return
		}
	}
	h.events <- HandshakeEvent{
		ServerName:         state.ServerName,
		NegotiatedProtocol: state.NegotiatedProtocol,
		DidResume:          state.DidResume,
		CipherSuite:        state.CipherSuite,
		Version:            state.Version,
	}
	close(h.ready)
	h.runReader()
}

func verifyPeerName(state cryptotls.ConnectionState, name string) error {
	if len(state.PeerCertificates) == 0 {
		return ErrPeerCertificateUnavailable
	}
	return state.PeerCertificates[0].VerifyHostname(name)
}

func (h *Handler) runReader() {
	var scratch [16 * 1024]byte
	for {
		n, err := h.conn.Read(scratch[:])
		if n > 0 {
			data := append([]byte(nil), scratch[:n]...)
			select {
			case h.plain <- data:
			case <-h.closed:
				return
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
			return
		}
		h.sendErr(err)
		return
	}
}

func (h *Handler) runWriter() {
	select {
	case <-h.ready:
	case <-h.closed:
		return
	}
	for {
		select {
		case data := <-h.app:
			if _, err := h.conn.Write(data); err != nil {
				h.sendErr(err)
				return
			}
		case <-h.closed:
			return
		}
	}
}

func (h *Handler) drain(ctx *channel.HandlerContext, flush bool, wait bool) {
	if !h.started.Load() || h.raw == nil {
		if flush {
			_ = ctx.Flush()
		}
		return
	}
	deadline := time.Now().Add(20 * time.Millisecond)
	for {
		drained := false
		select {
		case data := <-h.raw.out:
			drained = true
			if err := h.writeCipher(ctx, data); err != nil {
				h.fail(ctx, err)
				return
			}
		default:
		}
		select {
		case data := <-h.plain:
			drained = true
			if err := h.firePlain(ctx, data); err != nil {
				h.fail(ctx, err)
				return
			}
		default:
		}
		select {
		case event := <-h.events:
			drained = true
			h.handshake = true
			ctx.FireUserEventTriggered(event)
			if !h.active {
				h.active = true
				ctx.FireChannelActive()
			}
		default:
		}
		select {
		case err := <-h.errs:
			h.fail(ctx, err)
			return
		default:
		}
		if drained {
			continue
		}
		if !wait || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if flush {
		_ = ctx.Flush()
	}
}

func (h *Handler) writeCipher(ctx *channel.HandlerContext, data []byte) error {
	out, err := ctx.Channel().Allocator().Acquire(len(data))
	if err != nil {
		return err
	}
	if _, err := out.WriteBytes(data); err != nil {
		out.Release()
		return err
	}
	if err := ctx.Write(out); err != nil {
		out.Release()
		return err
	}
	return nil
}

func (h *Handler) firePlain(ctx *channel.HandlerContext, data []byte) error {
	out, err := ctx.Channel().Allocator().Acquire(len(data))
	if err != nil {
		return err
	}
	if _, err := out.WriteBytes(data); err != nil {
		out.Release()
		return err
	}
	ctx.FireChannelRead(out)
	ctx.FireChannelReadComplete()
	return nil
}

func (h *Handler) sendErr(err error) {
	select {
	case h.errs <- err:
	case <-h.closed:
	}
}

func (h *Handler) close() {
	h.closeOnce.Do(func() {
		close(h.closed)
		if h.conn != nil {
			_ = h.conn.Close()
		}
		if h.raw != nil {
			_ = h.raw.Close()
		}
	})
}

func (h *Handler) fail(ctx *channel.HandlerContext, err error) {
	ctx.FireExceptionCaught(err)
	_ = ctx.Close()
}

func release(msg any) {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		buf.Release()
		return
	}
	if releasable, ok := msg.(interface{ Release() }); ok {
		releasable.Release()
	}
}

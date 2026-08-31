package quic

import (
	"context"
	"net"

	nativequic "github.com/quic-go/quic-go"
)

type quicListener struct {
	inner *nativequic.Listener
}

type quicEarlyListener struct {
	inner *nativequic.EarlyListener
}

// ListenAddr 在 addr 上创建 RFC9000 QUIC v1 监听器。
func ListenAddr(addr string, cfg Config) (Listener, error) {
	if addr == "" {
		return nil, ErrMissingAddress
	}
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	inner, err := nativequic.ListenAddr(addr, normalized.tls, normalized.quic)
	if err != nil {
		return nil, err
	}
	return &quicListener{inner: inner}, nil
}

// ListenAddrEarly 在 addr 上创建允许 0-RTT 的 RFC9000 QUIC v1 监听器。
func ListenAddrEarly(addr string, cfg Config) (EarlyListener, error) {
	if addr == "" {
		return nil, ErrMissingAddress
	}
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	if err := validate0RTTServerConfig(normalized.public); err != nil {
		return nil, err
	}
	inner, err := nativequic.ListenAddrEarly(addr, normalized.tls, normalized.quic)
	if err != nil {
		return nil, err
	}
	return &quicEarlyListener{inner: inner}, nil
}

func (l *quicListener) Addr() net.Addr {
	if l == nil || l.inner == nil {
		return nil
	}
	return l.inner.Addr()
}

func (l *quicListener) Accept(ctx context.Context) (Connection, error) {
	if l == nil || l.inner == nil {
		return nil, ErrClosed
	}
	conn, err := l.inner.Accept(normalizeContext(ctx))
	if err != nil {
		return nil, err
	}
	return wrapConnection(conn), nil
}

func (l *quicListener) Close() error {
	if l == nil || l.inner == nil {
		return ErrClosed
	}
	return l.inner.Close()
}

func (l *quicEarlyListener) Addr() net.Addr {
	if l == nil || l.inner == nil {
		return nil
	}
	return l.inner.Addr()
}

func (l *quicEarlyListener) Accept(ctx context.Context) (Connection, error) {
	if l == nil || l.inner == nil {
		return nil, ErrClosed
	}
	conn, err := l.inner.Accept(normalizeContext(ctx))
	if err != nil {
		return nil, err
	}
	return wrapConnection(conn), nil
}

func (l *quicEarlyListener) Close() error {
	if l == nil || l.inner == nil {
		return ErrClosed
	}
	return l.inner.Close()
}

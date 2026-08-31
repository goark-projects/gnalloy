package quic

import (
	"context"
	"net"

	nativequic "github.com/quic-go/quic-go"
)

type quicConnection struct {
	inner *nativequic.Conn
}

func wrapConnection(conn *nativequic.Conn) Connection {
	if conn == nil {
		return nil
	}
	return &quicConnection{inner: conn}
}

func (c *quicConnection) LocalAddr() net.Addr {
	if c == nil || c.inner == nil {
		return nil
	}
	return c.inner.LocalAddr()
}

func (c *quicConnection) RemoteAddr() net.Addr {
	if c == nil || c.inner == nil {
		return nil
	}
	return c.inner.RemoteAddr()
}

func (c *quicConnection) HandshakeComplete() <-chan struct{} {
	if c == nil || c.inner == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return c.inner.HandshakeComplete()
}

func (c *quicConnection) ConnectionState() State {
	if c == nil || c.inner == nil {
		return State{}
	}
	return stateFromNative(c.inner.ConnectionState())
}

func (c *quicConnection) Stats() ConnectionStats {
	if c == nil || c.inner == nil {
		return ConnectionStats{}
	}
	return statsFromNative(c.inner.ConnectionStats())
}

func (c *quicConnection) OpenStreamSync(ctx context.Context) (Stream, error) {
	if c == nil || c.inner == nil {
		return nil, ErrClosed
	}
	stream, err := c.inner.OpenStreamSync(normalizeContext(ctx))
	if err != nil {
		return nil, err
	}
	return wrapStream(stream), nil
}

func (c *quicConnection) AcceptStream(ctx context.Context) (Stream, error) {
	if c == nil || c.inner == nil {
		return nil, ErrClosed
	}
	stream, err := c.inner.AcceptStream(normalizeContext(ctx))
	if err != nil {
		return nil, err
	}
	return wrapStream(stream), nil
}

func (c *quicConnection) OpenUniStreamSync(ctx context.Context) (SendStream, error) {
	if c == nil || c.inner == nil {
		return nil, ErrClosed
	}
	stream, err := c.inner.OpenUniStreamSync(normalizeContext(ctx))
	if err != nil {
		return nil, err
	}
	return wrapSendStream(stream), nil
}

func (c *quicConnection) AcceptUniStream(ctx context.Context) (ReceiveStream, error) {
	if c == nil || c.inner == nil {
		return nil, ErrClosed
	}
	stream, err := c.inner.AcceptUniStream(normalizeContext(ctx))
	if err != nil {
		return nil, err
	}
	return wrapReceiveStream(stream), nil
}

func (c *quicConnection) SendDatagram(payload []byte) error {
	if c == nil || c.inner == nil {
		return ErrClosed
	}
	return c.inner.SendDatagram(payload)
}

func (c *quicConnection) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	if c == nil || c.inner == nil {
		return nil, ErrClosed
	}
	return c.inner.ReceiveDatagram(normalizeContext(ctx))
}

func (c *quicConnection) CloseWithError(code ApplicationErrorCode, reason string) error {
	if c == nil || c.inner == nil {
		return ErrClosed
	}
	return c.inner.CloseWithError(nativequic.ApplicationErrorCode(code), reason)
}

func stateFromNative(state nativequic.ConnectionState) State {
	return State{
		TLS:     state.TLS,
		Version: Version(state.Version),
		SupportsDatagrams: FeatureSupport{
			Local:  state.SupportsDatagrams.Local,
			Remote: state.SupportsDatagrams.Remote,
		},
		SupportsStreamResetPartialDelivery: FeatureSupport{
			Local:  state.SupportsStreamResetPartialDelivery.Local,
			Remote: state.SupportsStreamResetPartialDelivery.Remote,
		},
		Used0RTT: state.Used0RTT,
		GSO:      state.GSO,
	}
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

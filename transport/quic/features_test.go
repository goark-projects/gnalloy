package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"testing"
	"time"
)

func TestNormalizeConfigEnables0RTTTokenStore(t *testing.T) {
	normalized, err := normalizeConfig(Config{
		TLS:        &tls.Config{},
		Enable0RTT: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !normalized.public.Enable0RTT {
		t.Fatal("0-RTT should remain enabled")
	}
	if normalized.public.ClientTokenStoreMaxOrigins != DefaultClientTokenStoreMaxOrigins {
		t.Fatalf("max origins=%d, want %d", normalized.public.ClientTokenStoreMaxOrigins, DefaultClientTokenStoreMaxOrigins)
	}
	if normalized.public.ClientTokenStoreTokensPerOrigin != DefaultClientTokenStoreTokensPerOrigin {
		t.Fatalf("tokens per origin=%d, want %d", normalized.public.ClientTokenStoreTokensPerOrigin, DefaultClientTokenStoreTokensPerOrigin)
	}
	if normalized.public.ClientTokenStore == nil {
		t.Fatal("missing normalized client token store")
	}
	if normalized.quic == nil || !normalized.quic.Allow0RTT || normalized.quic.TokenStore != normalized.public.ClientTokenStore {
		t.Fatalf("native quic config=%+v, want Allow0RTT and token store", normalized.quic)
	}
}

func TestNewClientTokenStoreRejectsInvalidCapacity(t *testing.T) {
	if _, err := NewClientTokenStore(0, 1); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v, want ErrInvalidConfig", err)
	}
	if _, err := NewClientTokenStore(1, 0); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v, want ErrInvalidConfig", err)
	}
}

func TestEvaluateCapabilitiesReportsWebTransportWhenEnabled(t *testing.T) {
	caps, err := EvaluateCapabilities(EndpointRoleClient, Config{
		TLS: &tls.Config{
			ClientSessionCache: tls.NewLRUClientSessionCache(8),
		},
		Enable0RTT:         true,
		EnableWebTransport: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !caps.RFC9000 || !caps.TLS13 {
		t.Fatalf("base capabilities=%+v", caps)
	}
	if !caps.SessionResumption.Supported || !caps.SessionResumption.Enabled {
		t.Fatalf("session resumption=%+v", caps.SessionResumption)
	}
	if !caps.ZeroRTT.Supported || !caps.ZeroRTT.Enabled {
		t.Fatalf("zero rtt=%+v", caps.ZeroRTT)
	}
	if !caps.Datagrams.Supported || !caps.Datagrams.Enabled {
		t.Fatalf("datagrams=%+v", caps.Datagrams)
	}
	if !caps.StreamResetPartialDelivery.Supported || !caps.StreamResetPartialDelivery.Enabled {
		t.Fatalf("stream reset partial delivery=%+v", caps.StreamResetPartialDelivery)
	}
	if !caps.WebTransport.Supported || !caps.WebTransport.Enabled {
		t.Fatalf("webtransport=%+v", caps.WebTransport)
	}
}

func TestNormalizeConfigEnablesWebTransportPrerequisites(t *testing.T) {
	cfg, err := NormalizeConfig(Config{TLS: &tls.Config{}, EnableWebTransport: true})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.EnableDatagrams || !cfg.EnableStreamResetPartialDelivery {
		t.Fatalf("datagrams=%v reset=%v, want enabled", cfg.EnableDatagrams, cfg.EnableStreamResetPartialDelivery)
	}
}

func TestDialAddrEarlyValidatesSessionBoundary(t *testing.T) {
	_, err := DialAddrEarly(context.Background(), "127.0.0.1:1", Config{
		TLS: &tls.Config{ClientSessionCache: tls.NewLRUClientSessionCache(1)},
	})
	if !errors.Is(err, Err0RTTDisabled) {
		t.Fatalf("err=%v, want Err0RTTDisabled", err)
	}

	_, err = DialAddrEarly(context.Background(), "127.0.0.1:1", Config{
		TLS:        &tls.Config{},
		Enable0RTT: true,
	})
	if !errors.Is(err, ErrMissingSessionCache) {
		t.Fatalf("err=%v, want ErrMissingSessionCache", err)
	}
}

func TestListenAddrEarlyValidatesServerBoundary(t *testing.T) {
	_, err := ListenAddrEarly("127.0.0.1:0", Config{TLS: &tls.Config{}})
	if !errors.Is(err, Err0RTTDisabled) {
		t.Fatalf("err=%v, want Err0RTTDisabled", err)
	}

	_, err = ListenAddrEarly("127.0.0.1:0", Config{
		TLS:        &tls.Config{SessionTicketsDisabled: true},
		Enable0RTT: true,
	})
	if !errors.Is(err, ErrInvalidTLSConfig) {
		t.Fatalf("err=%v, want ErrInvalidTLSConfig", err)
	}
}

func TestListenDialAddrEarlyUses0RTTAfterSessionResumption(t *testing.T) {
	const alpn = "gnalloy-0rtt-test"
	cert, roots := testCertificate(t, "gnalloy.local")
	listener, err := ListenAddrEarly("127.0.0.1:0", Config{
		TLS:        &tls.Config{Certificates: []tls.Certificate{cert}},
		NextProtos: []string{alpn},
		Enable0RTT: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cache := &notifyingClientSessionCache{
		inner: tls.NewLRUClientSessionCache(8),
		puts:  make(chan struct{}, 1),
	}
	clientTLS := &tls.Config{
		RootCAs:            roots,
		ServerName:         "gnalloy.local",
		ClientSessionCache: cache,
	}

	firstServerErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept(ctx)
		if err != nil {
			firstServerErr <- err
			return
		}
		firstServerErr <- waitQUICHandshake(ctx, conn)
	}()

	firstConn, err := DialAddr(ctx, listener.Addr().String(), Config{
		TLS:        clientTLS,
		NextProtos: []string{alpn},
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-cache.puts:
	case <-ctx.Done():
		t.Fatal("timeout waiting for TLS session ticket")
	}
	if err := firstConn.CloseWithError(0, "session cached"); err != nil {
		t.Fatal(err)
	}
	if err := <-firstServerErr; err != nil {
		t.Fatal(err)
	}

	serverState := make(chan State, 1)
	serverErr := make(chan error, 1)
	go func() {
		state, err := serve0RTTEcho(ctx, listener)
		if err != nil {
			serverErr <- err
			return
		}
		serverState <- state
		serverErr <- nil
	}()

	conn, err := DialAddrEarly(ctx, listener.Addr().String(), Config{
		TLS:        clientTLS,
		NextProtos: []string{alpn},
		Enable0RTT: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseWithError(0, "0-rtt done")

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	payload := []byte("early data")
	if _, err := stream.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("echo=%q, want %q", got, payload)
	}
	if err := waitQUICHandshake(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if !conn.ConnectionState().Used0RTT {
		t.Fatal("client did not use 0-RTT")
	}
	select {
	case state := <-serverState:
		if !state.Used0RTT {
			t.Fatal("server did not accept 0-RTT")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for server 0-RTT state")
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

type notifyingClientSessionCache struct {
	inner tls.ClientSessionCache
	puts  chan struct{}
}

func (c *notifyingClientSessionCache) Get(key string) (*tls.ClientSessionState, bool) {
	return c.inner.Get(key)
}

func (c *notifyingClientSessionCache) Put(key string, state *tls.ClientSessionState) {
	c.inner.Put(key, state)
	select {
	case c.puts <- struct{}{}:
	default:
	}
}

func waitQUICHandshake(ctx context.Context, conn Connection) error {
	select {
	case <-conn.HandshakeComplete():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func serve0RTTEcho(ctx context.Context, listener EarlyListener) (State, error) {
	conn, err := listener.Accept(ctx)
	if err != nil {
		return State{}, err
	}
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		return State{}, err
	}
	if err := stream.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return State{}, err
	}
	if _, err := io.Copy(stream, stream); err != nil {
		return State{}, err
	}
	if err := stream.Close(); err != nil {
		return State{}, err
	}
	if err := waitQUICHandshake(ctx, conn); err != nil {
		return State{}, err
	}
	return conn.ConnectionState(), nil
}

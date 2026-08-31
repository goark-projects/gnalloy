package benchh3

import (
	"context"
	"crypto/tls"
	"sync"

	h3transport "goark.dev/gnalloy/transport/http3"
	"goark.dev/gnalloy/transport/quic"
)

func prepareClients(ctx context.Context, cfg Config) ([]client, error) {
	clients := make([]client, 0, cfg.Connections)
	for i := 0; i < cfg.Connections; i++ {
		conn, err := dial(ctx, cfg)
		if err != nil {
			closeClients(clients)
			return nil, err
		}
		session, err := h3transport.NewSession(conn, h3transport.Config{})
		if err != nil {
			_ = conn.CloseWithError(0, "http3 session failed")
			closeClients(clients)
			return nil, err
		}
		clients = append(clients, client{
			conn:     conn,
			session:  session,
			headers:  requestHeaders(cfg.ServerName),
			expected: ResponseBody(cfg.Payload),
			reply:    make([]byte, cfg.Payload),
			alpn:     conn.ConnectionState().TLS.NegotiatedProtocol,
		})
	}
	return clients, nil
}

func dial(ctx context.Context, cfg Config) (quic.Connection, error) {
	tlsCfg := clientTLSConfig(cfg)
	return quic.DialAddr(ctx, cfg.Addr, quic.Config{
		TLS:        tlsCfg,
		NextProtos: []string{alpnHTTP3},
	})
}

func clientTLSConfig(cfg Config) *tls.Config {
	tlsCfg := cfg.TLS
	if tlsCfg == nil {
		tlsCfg = &tls.Config{InsecureSkipVerify: true}
	}
	out := tlsCfg.Clone()
	if out.ServerName == "" {
		out.ServerName = cfg.ServerName
	}
	if len(out.NextProtos) == 0 {
		out.NextProtos = []string{alpnHTTP3}
	}
	if out.MinVersion == 0 || out.MinVersion < tls.VersionTLS13 {
		out.MinVersion = tls.VersionTLS13
	}
	if out.MaxVersion == 0 {
		out.MaxVersion = tls.VersionTLS13
	}
	return out
}

func closeClients(clients []client) {
	for i := range clients {
		if clients[i].conn != nil {
			_ = clients[i].conn.CloseWithError(0, "benchmark done")
		}
	}
}

func closeClientsOnContext(ctx context.Context, clients []client) func() {
	done := make(chan struct{})
	var once sync.Once
	go func() {
		select {
		case <-ctx.Done():
			closeClients(clients)
		case <-done:
		}
	}()
	return func() {
		once.Do(func() {
			close(done)
		})
	}
}

func firstNegotiatedProtocol(clients []client) string {
	for i := range clients {
		if clients[i].alpn != "" {
			return clients[i].alpn
		}
	}
	return ""
}

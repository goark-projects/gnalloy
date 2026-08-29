package benchhttp

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type client struct {
	conn    net.Conn
	reader  *bufio.Reader
	request []byte
	body    []byte
	reply   []byte
	alpn    string
}

func prepareClients(ctx context.Context, cfg Config) ([]client, error) {
	clients := make([]client, 0, cfg.Connections)
	for i := 0; i < cfg.Connections; i++ {
		conn, alpn, err := dial(ctx, cfg)
		if err != nil {
			closeClients(clients)
			return nil, err
		}
		clients = append(clients, client{
			conn:    conn,
			reader:  bufio.NewReaderSize(conn, 16*1024),
			request: RequestBytes(cfg.ServerName),
			body:    ResponseBody(cfg.Payload),
			reply:   make([]byte, cfg.Payload),
			alpn:    alpn,
		})
	}
	return clients, nil
}

func dial(ctx context.Context, cfg Config) (net.Conn, string, error) {
	dialer := net.Dialer{Timeout: cfg.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", cfg.Addr)
	if err != nil {
		return nil, "", err
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}
	if err := conn.SetDeadline(time.Now().Add(cfg.Timeout)); err != nil {
		_ = conn.Close()
		return nil, "", err
	}
	if cfg.TLS == nil {
		return conn, "", nil
	}
	tlsCfg := cfg.TLS.Clone()
	if tlsCfg.ServerName == "" {
		tlsCfg.ServerName = cfg.ServerName
	}
	tlsConn := tls.Client(conn, tlsCfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, "", err
	}
	return tlsConn, tlsConn.ConnectionState().NegotiatedProtocol, nil
}

func warmupClients(ctx context.Context, clients []client, cfg Config) error {
	if cfg.WarmupMessages <= 0 {
		return nil
	}
	var (
		firstErr error
		once     sync.Once
		wg       sync.WaitGroup
	)
	recordError := func(err error) {
		if err == nil {
			return
		}
		once.Do(func() {
			firstErr = err
		})
	}
	startCh := make(chan struct{})
	for i := range clients {
		clientID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			recordError(runClientMessages(ctx, &clients[clientID], cfg.WarmupMessages, cfg.LatencySampleRate, startCh, nil, nil))
		}()
	}
	close(startCh)
	wg.Wait()
	return firstErr
}

func closeClients(clients []client) {
	for i := range clients {
		if clients[i].conn != nil {
			_ = clients[i].conn.Close()
		}
	}
}

func runClientMessages(
	ctx context.Context,
	c *client,
	messageCount int,
	latencySampleRate int,
	startCh <-chan struct{},
	successes *atomic.Int64,
	latencySamples *[]int64,
) error {
	select {
	case <-startCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	for i := 0; i < messageCount; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		recordLatency := latencySamples != nil && shouldRecordLatency(i, latencySampleRate)
		var requestStarted time.Time
		if recordLatency {
			requestStarted = time.Now()
		}
		if err := writeAll(c.conn, c.request); err != nil {
			return err
		}
		if err := readResponse(c.reader, c.reply, c.body); err != nil {
			return err
		}
		if recordLatency && latencySamples != nil {
			*latencySamples = append(*latencySamples, elapsedLatencyNanos(requestStarted))
		}
		if successes != nil {
			successes.Add(1)
		}
	}
	return nil
}

func firstNegotiatedProtocol(clients []client) string {
	for i := range clients {
		if clients[i].alpn != "" {
			return clients[i].alpn
		}
	}
	return ""
}

func writeAll(w io.Writer, src []byte) error {
	for len(src) > 0 {
		n, err := w.Write(src)
		if n > 0 {
			src = src[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

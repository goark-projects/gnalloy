package benchh2

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type client struct {
	conn        net.Conn
	reader      *bufio.Reader
	headerBlock []byte
	expected    []byte
	reply       []byte
	nextStream  uint32
	alpn        string
}

// RunLoad 执行 HTTP/2 stream request/response 负载。
func RunLoad(parent context.Context, cfg Config) (Result, error) {
	if err := cfg.Validate(); err != nil {
		return Result{}, err
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, cfg.Timeout)
	defer cancel()

	clients, err := prepareClients(ctx, cfg)
	if err != nil {
		return Result{Errors: 1}, err
	}
	stopClosing := closeClientsOnContext(ctx, clients)
	defer stopClosing()
	defer closeClients(clients)
	if err := warmupClients(ctx, clients, cfg); err != nil {
		return Result{Errors: 1}, err
	}

	var (
		successes atomic.Int64
		errorsN   atomic.Int64
		firstErr  error
		once      sync.Once
		wg        sync.WaitGroup
		samples   [][]int64
	)
	if latencySamplingEnabled(cfg.LatencySampleRate) {
		samples = make([][]int64, cfg.Connections)
	}
	recordError := func(err error) {
		if err == nil {
			return
		}
		errorsN.Add(1)
		once.Do(func() {
			firstErr = err
			cancel()
		})
	}

	startCh := make(chan struct{})
	for i := range clients {
		clientID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			var clientSamples *[]int64
			if samples != nil {
				samples[clientID] = newLatencySamples(cfg.Messages, cfg.LatencySampleRate)
				clientSamples = &samples[clientID]
			}
			recordError(runClientMessages(ctx, &clients[clientID], cfg.Messages, cfg.LatencySampleRate, startCh, &successes, clientSamples))
		}()
	}
	start := time.Now()
	close(startCh)
	wg.Wait()
	elapsed := time.Since(start)
	if elapsed <= 0 {
		elapsed = time.Nanosecond
	}

	total := successes.Load()
	result := Result{
		TotalRequests:      total,
		Errors:             errorsN.Load(),
		Elapsed:            elapsed,
		NegotiatedProtocol: firstNegotiatedProtocol(clients),
	}
	if samples != nil {
		allSamples := make([]int64, 0, estimateLatencySampleCount(cfg.Connections, cfg.Messages, cfg.LatencySampleRate))
		for _, clientSamples := range samples {
			allSamples = append(allSamples, clientSamples...)
		}
		result.Latency = summarizeLatencySamples(allSamples)
	}
	result.Throughput = float64(total) / elapsed.Seconds()
	if total > 0 {
		result.NsPerOp = float64(elapsed.Nanoseconds()) / float64(total)
	}
	if firstErr != nil {
		return result, firstErr
	}
	expected := int64(cfg.Connections * cfg.Messages)
	if total != expected {
		return result, fmt.Errorf("benchh2: completed %d requests, want %d", total, expected)
	}
	return result, nil
}

func prepareClients(ctx context.Context, cfg Config) ([]client, error) {
	clients := make([]client, 0, cfg.Connections)
	for i := 0; i < cfg.Connections; i++ {
		conn, alpn, err := dial(ctx, cfg)
		if err != nil {
			closeClients(clients)
			return nil, err
		}
		if err := sendClientPreface(conn); err != nil {
			_ = conn.Close()
			closeClients(clients)
			return nil, err
		}
		clients = append(clients, client{
			conn:        conn,
			reader:      bufio.NewReaderSize(conn, 16*1024),
			headerBlock: requestHeaderBlock(cfg.ServerName, cfg.TLS != nil),
			expected:    ResponseBody(cfg.Payload),
			reply:       make([]byte, cfg.Payload),
			nextStream:  1,
			alpn:        alpn,
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

func sendClientPreface(conn net.Conn) error {
	if err := writeAll(conn, clientPreface); err != nil {
		return err
	}
	if err := writeFrame(conn, frameSettings, 0, 0, nil); err != nil {
		return err
	}
	return writeConnectionWindowUpdate(conn)
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

func runClientMessages(ctx context.Context, c *client, messageCount int, latencySampleRate int, startCh <-chan struct{}, successes *atomic.Int64, latencySamples *[]int64) error {
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
		streamID := c.nextStream
		c.nextStream += 2
		recordLatency := latencySamples != nil && shouldRecordLatency(i, latencySampleRate)
		var requestStarted time.Time
		if recordLatency {
			requestStarted = time.Now()
		}
		if err := writeFrame(c.conn, frameHeaders, flagEndHeaders|flagEndStream, streamID, c.headerBlock); err != nil {
			return err
		}
		if err := readResponse(c, streamID); err != nil {
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

func readResponse(c *client, streamID uint32) error {
	received := 0
	for {
		header, err := readFrameHeader(c.reader)
		if err != nil {
			return err
		}
		switch header.typ {
		case frameSettings:
			if err := skipFramePayload(c.reader, header.length); err != nil {
				return err
			}
			if header.flags&flagAck == 0 {
				if err := writeSettingsAck(c.conn); err != nil {
					return err
				}
			}
		case framePing:
			var payload [8]byte
			if header.length != len(payload) {
				return fmt.Errorf("benchh2: invalid ping length %d", header.length)
			}
			if _, err := io.ReadFull(c.reader, payload[:]); err != nil {
				return err
			}
			if header.flags&flagAck == 0 {
				if err := writeFrame(c.conn, framePing, flagAck, 0, payload[:]); err != nil {
					return err
				}
			}
		case frameHeaders:
			if err := skipFramePayload(c.reader, header.length); err != nil {
				return err
			}
		case frameData:
			if header.streamID != streamID {
				if err := skipFramePayload(c.reader, header.length); err != nil {
					return err
				}
				continue
			}
			if header.length > len(c.reply)-received {
				return fmt.Errorf("benchh2: response body too large")
			}
			if _, err := io.ReadFull(c.reader, c.reply[received:received+header.length]); err != nil {
				return err
			}
			received += header.length
			if header.flags&flagEndStream == 0 {
				continue
			}
			if received != len(c.expected) {
				return fmt.Errorf("benchh2: response body length %d, want %d", received, len(c.expected))
			}
			if !bytes.Equal(c.reply[:received], c.expected) {
				return fmt.Errorf("benchh2: response body mismatch")
			}
			return nil
		default:
			if err := skipFramePayload(c.reader, header.length); err != nil {
				return err
			}
		}
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

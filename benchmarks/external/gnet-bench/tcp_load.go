package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

func runTCPEchoLoad(parent context.Context, addr string, cfg config) (benchResult, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	clients, err := prepareTCPClients(ctx, addr, cfg)
	if err != nil {
		return benchResult{Errors: 1}, err
	}
	defer closeTCPClients(clients)
	if err := warmupTCPClients(ctx, clients, cfg); err != nil {
		return benchResult{Errors: 1}, err
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
			recordError(runClient(ctx, clients[clientID], cfg, clientID, startCh, &successes, clientSamples))
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
	result := benchResult{TotalRequests: total, Errors: errorsN.Load(), Elapsed: elapsed}
	if samples != nil {
		allSamples := make([]int64, 0, estimateLatencySampleCount(cfg.Connections, cfg.Messages, cfg.LatencySampleRate))
		for _, clientSamples := range samples {
			allSamples = append(allSamples, clientSamples...)
		}
		result.Latency = summarizeLatencySamples(allSamples)
	}
	if elapsed > 0 {
		result.Throughput = float64(total) / elapsed.Seconds()
	}
	if total > 0 {
		result.NsPerOp = float64(elapsed.Nanoseconds()) / float64(total)
	}
	if firstErr != nil {
		return result, firstErr
	}
	expected := int64(cfg.Connections * cfg.Messages)
	if total != expected {
		return result, fmt.Errorf("gnet-bench: completed %d requests, want %d", total, expected)
	}
	return result, nil
}

type tcpClient struct {
	conn    net.Conn
	payload []byte
	reply   []byte
}

func prepareTCPClients(ctx context.Context, addr string, cfg config) ([]tcpClient, error) {
	clients := make([]tcpClient, 0, cfg.Connections)
	for i := 0; i < cfg.Connections; i++ {
		conn, err := dialTCPClient(ctx, addr, cfg)
		if err != nil {
			closeTCPClients(clients)
			return nil, err
		}
		clients = append(clients, tcpClient{
			conn:    conn,
			payload: makePayload(cfg.Payload, i),
			reply:   make([]byte, cfg.Payload),
		})
	}
	return clients, nil
}

func warmupTCPClients(ctx context.Context, clients []tcpClient, cfg config) error {
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
			recordError(runClientMessages(ctx, clients[clientID], cfg, clientID, startCh, cfg.WarmupMessages, nil, nil))
		}()
	}
	close(startCh)
	wg.Wait()
	return firstErr
}

func dialTCPClient(ctx context.Context, addr string, cfg config) (net.Conn, error) {
	dialer := net.Dialer{Timeout: cfg.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}
	if err := conn.SetDeadline(time.Now().Add(cfg.Timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func closeTCPClients(clients []tcpClient) {
	for i := range clients {
		if clients[i].conn != nil {
			_ = clients[i].conn.Close()
		}
	}
}

func runClient(ctx context.Context, client tcpClient, cfg config, clientID int, startCh <-chan struct{}, successes *atomic.Int64, latencySamples *[]int64) error {
	return runClientMessages(ctx, client, cfg, clientID, startCh, cfg.Messages, successes, latencySamples)
}

func runClientMessages(ctx context.Context, client tcpClient, cfg config, clientID int, startCh <-chan struct{}, messageCount int, successes *atomic.Int64, latencySamples *[]int64) error {
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
		client.payload[0] = byte(clientID + i)
		recordLatency := latencySamples != nil && shouldRecordLatency(i, cfg.LatencySampleRate)
		var requestStarted time.Time
		if recordLatency {
			requestStarted = time.Now()
		}
		if err := writeAll(client.conn, client.payload); err != nil {
			return err
		}
		if _, err := io.ReadFull(client.conn, client.reply); err != nil {
			return err
		}
		if !bytes.Equal(client.reply, client.payload) {
			return fmt.Errorf("gnet-bench: echo mismatch")
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

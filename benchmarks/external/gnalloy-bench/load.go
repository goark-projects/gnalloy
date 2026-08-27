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

type benchResult struct {
	TotalRequests int64
	Errors        int64
	Elapsed       time.Duration
	Throughput    float64
	NsPerOp       float64
}

func runBenchmark(parent context.Context, cfg config) (benchResult, error) {
	if err := cfg.validate(); err != nil {
		return benchResult{}, err
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, cfg.Timeout)
	defer cancel()

	server, err := startEchoServer(ctx, cfg)
	if err != nil {
		return benchResult{}, err
	}
	defer server.stop()

	result, err := runTCPEchoLoad(ctx, server.addr, cfg)
	if err != nil {
		return result, err
	}
	return result, nil
}

func runTCPEchoLoad(parent context.Context, addr string, cfg config) (benchResult, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	var (
		successes atomic.Int64
		errorsN   atomic.Int64
		firstErr  error
		once      sync.Once
		wg        sync.WaitGroup
	)
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

	start := time.Now()
	for i := 0; i < cfg.Connections; i++ {
		clientID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			recordError(runClient(ctx, addr, cfg, clientID, &successes))
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	total := successes.Load()
	result := benchResult{TotalRequests: total, Errors: errorsN.Load(), Elapsed: elapsed}
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
		return result, fmt.Errorf("gnalloy-bench: completed %d requests, want %d", total, expected)
	}
	return result, nil
}

func runClient(ctx context.Context, addr string, cfg config, clientID int, successes *atomic.Int64) error {
	dialer := net.Dialer{Timeout: cfg.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}
	if err := conn.SetDeadline(time.Now().Add(cfg.Timeout)); err != nil {
		return err
	}

	payload := makePayload(cfg.Payload, clientID)
	reply := make([]byte, cfg.Payload)
	for i := 0; i < cfg.Messages; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		payload[0] = byte(clientID + i)
		if err := writeAll(conn, payload); err != nil {
			return err
		}
		if _, err := io.ReadFull(conn, reply); err != nil {
			return err
		}
		if !bytes.Equal(reply, payload) {
			return fmt.Errorf("gnalloy-bench: echo mismatch")
		}
		successes.Add(1)
	}
	return nil
}

func makePayload(size int, clientID int) []byte {
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(clientID + i)
	}
	return payload
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

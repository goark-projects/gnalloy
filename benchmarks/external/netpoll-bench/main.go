package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const benchmarkName = "BenchmarkNetpollTCPEcho"

var (
	errInvalidConfig       = errors.New("netpoll-bench: invalid config")
	errUnsupportedProtocol = errors.New("netpoll-bench: unsupported protocol")
	errUnsupportedPlatform = errors.New("netpoll-bench: unsupported platform")
)

type config struct {
	Protocol    string
	Addr        string
	Payload     int
	Connections int
	Messages    int
	Timeout     time.Duration
}

type benchResult struct {
	TotalRequests int64
	Errors        int64
	Elapsed       time.Duration
	Throughput    float64
	NsPerOp       float64
}

func main() {
	if err := runCLI(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCLI(args []string, stdout io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	result, err := runBenchmark(context.Background(), cfg)
	if result.TotalRequests > 0 {
		writeBenchmarkResult(stdout, cfg, result)
	}
	return err
}

func parseConfig(args []string) (config, error) {
	cfg := config{
		Protocol:    "tcp-echo",
		Addr:        "127.0.0.1:0",
		Payload:     1024,
		Connections: 256,
		Messages:    100000,
		Timeout:     5 * time.Minute,
	}
	fs := flag.NewFlagSet("netpoll-bench", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.Protocol, "protocol", cfg.Protocol, "benchmark protocol")
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.IntVar(&cfg.Payload, "payload", cfg.Payload, "payload bytes")
	fs.IntVar(&cfg.Connections, "connections", cfg.Connections, "concurrent connections")
	fs.IntVar(&cfg.Messages, "messages", cfg.Messages, "messages per connection")
	fs.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "overall timeout")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	return cfg, cfg.validate()
}

func (c config) validate() error {
	if strings.TrimSpace(c.Protocol) != "tcp-echo" {
		return fmt.Errorf("%w: %s", errUnsupportedProtocol, c.Protocol)
	}
	if strings.TrimSpace(c.Addr) == "" {
		return fmt.Errorf("%w: empty addr", errInvalidConfig)
	}
	if c.Payload <= 0 || c.Connections <= 0 || c.Messages <= 0 {
		return fmt.Errorf("%w: payload, connections and messages must be positive", errInvalidConfig)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("%w: timeout must be positive", errInvalidConfig)
	}
	return nil
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
		return result, fmt.Errorf("netpoll-bench: completed %d requests, want %d", total, expected)
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
			return fmt.Errorf("netpoll-bench: echo mismatch")
		}
		successes.Add(1)
	}
	return nil
}

func writeBenchmarkResult(w io.Writer, cfg config, result benchResult) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "framework=netpoll protocol=%s payload=%d connections=%d messages=%d total=%d errors=%d elapsed=%s throughput=%.2f ops/s\n",
		cfg.Protocol, cfg.Payload, cfg.Connections, cfg.Messages, result.TotalRequests, result.Errors, result.Elapsed, result.Throughput)
	fmt.Fprintf(w, "%s-%d %d %.0f ns/op\n", benchmarkName, runtime.GOMAXPROCS(0), result.TotalRequests, result.NsPerOp)
}

func resolveListenAddress(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	if port != "0" {
		return addr, nil
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return "", err
	}
	actual := ln.Addr().String()
	if err := ln.Close(); err != nil {
		return "", err
	}
	return actual, nil
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

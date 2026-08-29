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
	Protocol          string
	Addr              string
	Payload           int
	Connections       int
	Messages          int
	Timeout           time.Duration
	LatencySampleRate int
	WarmupMessages    int
}

type benchResult struct {
	TotalRequests int64
	Errors        int64
	Elapsed       time.Duration
	Throughput    float64
	NsPerOp       float64
	Latency       latencySummary
	Resources     resourceDelta
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
	fs.IntVar(&cfg.LatencySampleRate, "latency-sample-rate", cfg.LatencySampleRate, "record one round-trip latency sample every N messages per connection; 0 disables latency sampling")
	fs.IntVar(&cfg.WarmupMessages, "warmup-messages", cfg.WarmupMessages, "messages per connection sent before timed measurement; 0 disables in-process warmup")
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
	if c.LatencySampleRate < 0 {
		return fmt.Errorf("%w: latency-sample-rate must not be negative", errInvalidConfig)
	}
	if c.WarmupMessages < 0 {
		return fmt.Errorf("%w: warmup-messages must not be negative", errInvalidConfig)
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

	resourcesBefore := captureResourceSnapshot()
	result, err := runTCPEchoLoad(ctx, server.addr, cfg)
	result.Resources = resourceDeltaSince(resourcesBefore, captureResourceSnapshot())
	if err != nil {
		return result, err
	}
	return result, nil
}

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
		return result, fmt.Errorf("netpoll-bench: completed %d requests, want %d", total, expected)
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
			return fmt.Errorf("netpoll-bench: echo mismatch")
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

func estimateLatencySampleCount(connections int, messages int, rate int) int {
	if connections <= 0 || messages <= 0 || rate <= 0 {
		return 0
	}
	perConnection := messages / rate
	if messages%rate != 0 {
		perConnection++
	}
	return connections * perConnection
}

func writeBenchmarkResult(w io.Writer, cfg config, result benchResult) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "framework=netpoll protocol=%s backend=poller latencySampleRate=%d warmupMessages=%d latencySamples=%d p50LatencyNs=%d p95LatencyNs=%d p99LatencyNs=%d p999LatencyNs=%d maxLatencyNs=%d rssBytes=%d heapAllocBytes=%d heapSysBytes=%d heapObjects=%d gcCount=%d gcPauseNs=%d goroutines=%d payload=%d connections=%d messages=%d total=%d errors=%d elapsed=%s throughput=%.2f ops/s\n",
		cfg.Protocol, cfg.LatencySampleRate, cfg.WarmupMessages, result.Latency.Samples, result.Latency.P50.Nanoseconds(), result.Latency.P95.Nanoseconds(), result.Latency.P99.Nanoseconds(), result.Latency.P999.Nanoseconds(), result.Latency.Max.Nanoseconds(), result.Resources.RSSBytes, result.Resources.HeapAllocBytes, result.Resources.HeapSysBytes, result.Resources.HeapObjects, result.Resources.GCCount, result.Resources.GCPauseNanos, result.Resources.Goroutines, cfg.Payload, cfg.Connections, cfg.Messages, result.TotalRequests, result.Errors, result.Elapsed, result.Throughput)
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

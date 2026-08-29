package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

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
	switch strings.TrimSpace(c.Protocol) {
	case "tcp-echo", "http1":
	default:
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
	result, err := runLoad(ctx, server.addr, cfg)
	result.Resources = resourceDeltaSince(resourcesBefore, captureResourceSnapshot())
	if err != nil {
		return result, err
	}
	return result, nil
}

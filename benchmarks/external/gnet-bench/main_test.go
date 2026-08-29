package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-protocol", "tcp-echo",
		"-addr", "127.0.0.1:0",
		"-payload", "32",
		"-connections", "2",
		"-messages", "3",
		"-timeout", "2s",
		"-event-loops", "1",
		"-latency-sample-rate", "2",
		"-warmup-messages", "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Payload != 32 || cfg.Connections != 2 || cfg.Messages != 3 || cfg.Timeout != 2*time.Second || cfg.EventLoops != 1 {
		t.Fatalf("cfg=%+v", cfg)
	}
	if cfg.LatencySampleRate != 2 {
		t.Fatalf("latencySampleRate=%d, want 2", cfg.LatencySampleRate)
	}
	if cfg.WarmupMessages != 7 {
		t.Fatalf("warmupMessages=%d, want 7", cfg.WarmupMessages)
	}
}

func TestParseConfigRejectsUnsupportedProtocol(t *testing.T) {
	_, err := parseConfig([]string{"-protocol", "sctp-echo"})
	if !errors.Is(err, errUnsupportedProtocol) {
		t.Fatalf("err=%v, want %v", err, errUnsupportedProtocol)
	}
}

func TestParseConfigSupportsUDPEcho(t *testing.T) {
	cfg, err := parseConfig([]string{"-protocol", "udp-echo"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Protocol != "udp-echo" {
		t.Fatalf("protocol=%q, want udp-echo", cfg.Protocol)
	}
}

func TestParseConfigRejectsNegativeLatencySampleRate(t *testing.T) {
	_, err := parseConfig([]string{"-latency-sample-rate", "-1"})
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, errInvalidConfig)
	}
}

func TestParseConfigRejectsNegativeWarmupMessages(t *testing.T) {
	_, err := parseConfig([]string{"-warmup-messages", "-1"})
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, errInvalidConfig)
	}
}

func TestWriteBenchmarkResult(t *testing.T) {
	var out bytes.Buffer
	writeBenchmarkResult(&out, config{Protocol: "tcp-echo", Payload: 8, Connections: 1, Messages: 2, EventLoops: 4, LatencySampleRate: 2, WarmupMessages: 3}, benchResult{
		TotalRequests: 2,
		Elapsed:       4 * time.Microsecond,
		Throughput:    500000,
		NsPerOp:       2000,
		Latency: latencySummary{
			Samples: 1,
			P50:     time.Microsecond,
			P99:     2 * time.Microsecond,
		},
		Resources: resourceDelta{
			HeapAllocBytes: 1024,
			GCCount:        1,
			Goroutines:     2,
		},
	})
	text := out.String()
	for _, want := range []string{"framework=gnet", "backend=poller", "eventLoops=4", "latencySampleRate=2", "warmupMessages=3", "latencySamples=1", "p50LatencyNs=1000", "p99LatencyNs=2000", "heapAllocBytes=1024", "gcCount=1", "goroutines=2", "total=2", "BenchmarkGnetTCPEcho-", "2 2000 ns/op"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}

func TestRunBenchmarkTCPEcho(t *testing.T) {
	cfg := config{
		Protocol:          "tcp-echo",
		Addr:              "127.0.0.1:0",
		Payload:           16,
		Connections:       1,
		Messages:          2,
		Timeout:           5 * time.Second,
		EventLoops:        1,
		LatencySampleRate: 1,
		WarmupMessages:    1,
	}
	result, err := runBenchmark(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 2 || result.Errors != 0 || result.NsPerOp <= 0 {
		t.Fatalf("result=%+v", result)
	}
	if result.Latency.Samples != 2 || result.Latency.P50 <= 0 || result.Resources.Goroutines <= 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestRunBenchmarkUDPEcho(t *testing.T) {
	cfg := config{
		Protocol:          "udp-echo",
		Addr:              "127.0.0.1:0",
		Payload:           16,
		Connections:       1,
		Messages:          2,
		Timeout:           5 * time.Second,
		EventLoops:        1,
		LatencySampleRate: 1,
		WarmupMessages:    1,
	}
	result, err := runBenchmark(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 2 || result.Errors != 0 || result.NsPerOp <= 0 {
		t.Fatalf("result=%+v", result)
	}
	if result.Latency.Samples != 2 || result.Latency.P50 <= 0 || result.Resources.Goroutines <= 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestRunBenchmarkHTTP1(t *testing.T) {
	cfg := config{
		Protocol:          "http1",
		Addr:              "127.0.0.1:0",
		Payload:           16,
		Connections:       1,
		Messages:          2,
		Timeout:           5 * time.Second,
		EventLoops:        1,
		LatencySampleRate: 1,
		WarmupMessages:    1,
	}
	result, err := runBenchmark(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 2 || result.Errors != 0 || result.NsPerOp <= 0 {
		t.Fatalf("result=%+v", result)
	}
	if result.Latency.Samples != 2 || result.Latency.P50 <= 0 || result.Resources.Goroutines <= 0 {
		t.Fatalf("result=%+v", result)
	}
}

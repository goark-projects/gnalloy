package main

import (
	"bytes"
	"context"
	"errors"
	"runtime"
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
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Payload != 32 || cfg.Connections != 2 || cfg.Messages != 3 || cfg.Timeout != 2*time.Second {
		t.Fatalf("cfg=%+v", cfg)
	}
}

func TestParseConfigRejectsUnsupportedProtocol(t *testing.T) {
	_, err := parseConfig([]string{"-protocol", "udp-echo"})
	if !errors.Is(err, errUnsupportedProtocol) {
		t.Fatalf("err=%v, want %v", err, errUnsupportedProtocol)
	}
}

func TestWriteBenchmarkResult(t *testing.T) {
	var out bytes.Buffer
	writeBenchmarkResult(&out, config{Protocol: "tcp-echo", Payload: 8, Connections: 1, Messages: 2}, benchResult{
		TotalRequests: 2,
		Elapsed:       4 * time.Microsecond,
		Throughput:    500000,
		NsPerOp:       2000,
	})
	text := out.String()
	for _, want := range []string{"framework=netpoll", "backend=poller", "total=2", "BenchmarkNetpollTCPEcho-", "2 2000 ns/op"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}

func TestRunBenchmarkTCPEcho(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("CloudWeGo netpoll v0.7.5 在 Windows 上游为空实现")
	}
	cfg := config{
		Protocol:    "tcp-echo",
		Addr:        "127.0.0.1:0",
		Payload:     16,
		Connections: 1,
		Messages:    2,
		Timeout:     5 * time.Second,
	}
	result, err := runBenchmark(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 2 || result.Errors != 0 || result.NsPerOp <= 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestRunBenchmarkReportsUnsupportedPlatform(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅校验 Windows 上游空实现保护")
	}
	_, err := runBenchmark(context.Background(), config{
		Protocol:    "tcp-echo",
		Addr:        "127.0.0.1:0",
		Payload:     16,
		Connections: 1,
		Messages:    1,
		Timeout:     time.Second,
	})
	if !errors.Is(err, errUnsupportedPlatform) {
		t.Fatalf("err=%v, want %v", err, errUnsupportedPlatform)
	}
}

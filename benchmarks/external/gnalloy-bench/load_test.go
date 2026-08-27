package main

import (
	"context"
	"testing"
	"time"
)

func TestRunBenchmarkTCPEcho(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-protocol", "tcp-echo",
		"-addr", "127.0.0.1:0",
		"-payload", "16",
		"-connections", "1",
		"-messages", "2",
		"-timeout", "5s",
		"-workers", "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runBenchmark(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 2 || result.Errors != 0 || result.NsPerOp <= 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestRunBenchmarkRejectsInvalidConfig(t *testing.T) {
	cfg := config{Protocol: "tcp-echo", Addr: "127.0.0.1:0", Payload: 1, Connections: 1, Messages: 1, Timeout: time.Second}
	_, err := runBenchmark(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error")
	}
}

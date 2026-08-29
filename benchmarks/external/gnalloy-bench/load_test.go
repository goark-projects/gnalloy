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
		"-latency-sample-rate", "1",
		"-warmup-messages", "1",
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
	if result.Latency.Samples != 2 || result.Latency.P50 <= 0 || result.Resources.HeapSysBytes == 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestRunBenchmarkHTTP1(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-protocol", "http1",
		"-addr", "127.0.0.1:0",
		"-payload", "16",
		"-connections", "1",
		"-messages", "2",
		"-timeout", "5s",
		"-workers", "1",
		"-latency-sample-rate", "1",
		"-warmup-messages", "1",
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
	if result.Latency.Samples != 2 || result.Latency.P50 <= 0 || result.Resources.HeapSysBytes == 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestRunBenchmarkHTTPS1(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-protocol", "https1",
		"-addr", "127.0.0.1:0",
		"-payload", "16",
		"-connections", "1",
		"-messages", "2",
		"-timeout", "5s",
		"-workers", "1",
		"-latency-sample-rate", "1",
		"-warmup-messages", "1",
		"-alpn", "http/1.1",
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
	if result.Protocol != "http/1.1" {
		t.Fatalf("negotiatedProtocol=%q, want http/1.1", result.Protocol)
	}
}

func TestRunBenchmarkHTTP2(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-protocol", "http2",
		"-addr", "127.0.0.1:0",
		"-payload", "16",
		"-connections", "1",
		"-messages", "2",
		"-timeout", "5s",
		"-workers", "1",
		"-latency-sample-rate", "1",
		"-warmup-messages", "1",
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
	if result.Latency.Samples != 2 || result.Latency.P50 <= 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestRunBenchmarkHTTPS2ALPN(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-protocol", "https2",
		"-addr", "127.0.0.1:0",
		"-payload", "8",
		"-connections", "1",
		"-messages", "2",
		"-timeout", "5s",
		"-workers", "1",
		"-latency-sample-rate", "1",
		"-warmup-messages", "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runBenchmark(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 2 || result.Errors != 0 || result.Protocol != "h2" {
		t.Fatalf("result=%+v", result)
	}
}

func TestRunBenchmarkHTTPS2SustainedLoad(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-protocol", "https2",
		"-addr", "127.0.0.1:0",
		"-payload", "128",
		"-connections", "8",
		"-messages", "1000",
		"-timeout", "20s",
		"-workers", "4",
		"-latency-sample-rate", "64",
		"-warmup-messages", "100",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runBenchmark(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 8000 || result.Errors != 0 || result.Protocol != "h2" {
		t.Fatalf("result=%+v", result)
	}
}

func TestRunBenchmarkUDPEcho(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-protocol", "udp-echo",
		"-addr", "127.0.0.1:0",
		"-payload", "16",
		"-connections", "1",
		"-messages", "2",
		"-timeout", "5s",
		"-workers", "1",
		"-latency-sample-rate", "1",
		"-warmup-messages", "1",
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
	if result.Latency.Samples != 2 || result.Latency.P50 <= 0 {
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

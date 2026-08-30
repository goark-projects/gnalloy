package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"goark.dev/gnalloy/transport"
)

func TestWriteBenchmarkResult(t *testing.T) {
	var out bytes.Buffer
	writeBenchmarkResult(&out, config{
		Protocol:          "tcp-echo",
		TLSVersion:        "1.2",
		Backend:           transport.BackendStd,
		Boss:              1,
		Workers:           2,
		Payload:           8,
		Connections:       1,
		Messages:          2,
		LatencySampleRate: 1,
		WarmupMessages:    3,
	}, benchResult{
		TotalRequests: 2,
		Elapsed:       4 * time.Microsecond,
		Throughput:    500000,
		NsPerOp:       2000,
		Protocol:      "http/1.1",
		Latency: latencySummary{
			Samples: 2,
			P50:     100 * time.Microsecond,
			P95:     200 * time.Microsecond,
			P99:     200 * time.Microsecond,
			P999:    200 * time.Microsecond,
			Max:     200 * time.Microsecond,
		},
		Resources: resourceDelta{
			RSSBytes:       4096,
			HeapAllocBytes: 2048,
			HeapSysBytes:   8192,
			HeapObjects:    16,
			GCCount:        1,
			GCPauseNanos:   32,
			Goroutines:     4,
		},
	})
	text := out.String()
	for _, want := range []string{"framework=gnalloy", "backend=std", "boss=1", "workers=2", "tlsVersion=1.2", "negotiatedProtocol=http/1.1", "latencySampleRate=1", "warmupMessages=3", "latencySamples=2", "p99LatencyNs=200000", "rssBytes=4096", "gcCount=1", "total=2", "BenchmarkGnalloyTCPEcho-", "2 2000 ns/op"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}

func TestWriteHTTP3BenchmarkResultUsesRFC9000Backend(t *testing.T) {
	var out bytes.Buffer
	writeBenchmarkResult(&out, config{
		Protocol:    "http3",
		TLSVersion:  "1.3",
		Backend:     transport.BackendEpoll,
		Boss:        1,
		Workers:     4,
		Payload:     8,
		Connections: 1,
		Messages:    1,
	}, benchResult{
		TotalRequests: 1,
		Elapsed:       time.Microsecond,
		Throughput:    1000000,
		NsPerOp:       1000,
		Protocol:      "h3",
	})
	text := out.String()
	for _, want := range []string{"protocol=http3", "backend=rfc9000", "negotiatedProtocol=h3", "BenchmarkGnalloyHTTP3-"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}

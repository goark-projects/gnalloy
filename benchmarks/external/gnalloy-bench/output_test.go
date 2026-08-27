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
		Protocol:    "tcp-echo",
		Backend:     transport.BackendStd,
		Payload:     8,
		Connections: 1,
		Messages:    2,
	}, benchResult{
		TotalRequests: 2,
		Elapsed:       4 * time.Microsecond,
		Throughput:    500000,
		NsPerOp:       2000,
	})
	text := out.String()
	for _, want := range []string{"framework=gnalloy", "backend=std", "total=2", "BenchmarkGnalloyTCPEcho-", "2 2000 ns/op"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}

package main

import (
	"fmt"
	"io"
	"runtime"
)

func writeBenchmarkResult(w io.Writer, cfg config, result benchResult) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "framework=gnalloy protocol=%s backend=%s http1Mode=%s boss=%d workers=%d readBufferSize=%d reuseport=%t mmap=%t mmapBlockSize=%d mmapBlocks=%d iouringFixedBuffers=%t iouringMultishotAccept=%t iouringSQPoll=%t tlsVersion=%s cipherSuites=%s negotiatedProtocol=%s latencySampleRate=%d warmupMessages=%d latencySamples=%d p50LatencyNs=%d p95LatencyNs=%d p99LatencyNs=%d p999LatencyNs=%d maxLatencyNs=%d rssBytes=%d heapAllocBytes=%d heapSysBytes=%d heapObjects=%d gcCount=%d gcPauseNs=%d goroutines=%d payload=%d connections=%d messages=%d total=%d errors=%d elapsed=%s throughput=%.2f ops/s\n",
		cfg.Protocol, benchmarkBackendLabel(cfg), cfg.HTTP1Mode, cfg.Boss, cfg.Workers, cfg.ReadBufferSize, cfg.ReusePort, cfg.Mmap, cfg.MmapBlockSize, cfg.MmapBlocks, cfg.IOUringFixedBuffers, cfg.IOUringMultishotAccept, cfg.IOUringSQPoll, cfg.TLSVersion, cfg.CipherSuites, result.Protocol, cfg.LatencySampleRate, cfg.WarmupMessages, result.Latency.Samples, result.Latency.P50.Nanoseconds(), result.Latency.P95.Nanoseconds(), result.Latency.P99.Nanoseconds(), result.Latency.P999.Nanoseconds(), result.Latency.Max.Nanoseconds(), result.Resources.RSSBytes, result.Resources.HeapAllocBytes, result.Resources.HeapSysBytes, result.Resources.HeapObjects, result.Resources.GCCount, result.Resources.GCPauseNanos, result.Resources.Goroutines, cfg.Payload, cfg.Connections, cfg.Messages, result.TotalRequests, result.Errors, result.Elapsed, result.Throughput)
	fmt.Fprintf(w, "%s-%d %d %.0f ns/op\n", benchmarkName(cfg.Protocol), runtime.GOMAXPROCS(0), result.TotalRequests, result.NsPerOp)
}

func benchmarkBackendLabel(cfg config) string {
	if cfg.Protocol == "http3" {
		return "rfc9000"
	}
	return backendLabel(cfg.Backend)
}

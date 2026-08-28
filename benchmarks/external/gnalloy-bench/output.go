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
	fmt.Fprintf(w, "framework=gnalloy protocol=%s backend=%s boss=%d workers=%d readBufferSize=%d reuseport=%t mmap=%t mmapBlockSize=%d mmapBlocks=%d iouringFixedBuffers=%t iouringMultishotAccept=%t iouringSQPoll=%t payload=%d connections=%d messages=%d total=%d errors=%d elapsed=%s throughput=%.2f ops/s\n",
		cfg.Protocol, backendLabel(cfg.Backend), cfg.Boss, cfg.Workers, cfg.ReadBufferSize, cfg.ReusePort, cfg.Mmap, cfg.MmapBlockSize, cfg.MmapBlocks, cfg.IOUringFixedBuffers, cfg.IOUringMultishotAccept, cfg.IOUringSQPoll, cfg.Payload, cfg.Connections, cfg.Messages, result.TotalRequests, result.Errors, result.Elapsed, result.Throughput)
	fmt.Fprintf(w, "%s-%d %d %.0f ns/op\n", benchmarkName, runtime.GOMAXPROCS(0), result.TotalRequests, result.NsPerOp)
}

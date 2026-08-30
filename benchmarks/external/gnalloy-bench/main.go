package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

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

func benchmarkName(protocol string) string {
	switch protocol {
	case "http1":
		return "BenchmarkGnalloyHTTP1"
	case "https1":
		return "BenchmarkGnalloyHTTPS1"
	case "http2":
		return "BenchmarkGnalloyHTTP2"
	case "https2":
		return "BenchmarkGnalloyHTTPS2"
	case "http3":
		return "BenchmarkGnalloyHTTP3"
	case "udp-echo":
		return "BenchmarkGnalloyUDPEcho"
	default:
		return "BenchmarkGnalloyTCPEcho"
	}
}

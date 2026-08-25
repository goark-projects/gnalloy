package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"goark.dev/gnalloy/examples/internal/benchclient"
)

func main() {
	var (
		addr        string
		protocol    string
		connections int
		messages    int
		payloadSize int
		timeout     time.Duration
	)
	flag.StringVar(&addr, "addr", "127.0.0.1:9000", "server address")
	flag.StringVar(&protocol, "protocol", string(benchclient.ProtocolRaw), "protocol: raw, length-field, line, or fixed")
	flag.IntVar(&connections, "connections", 64, "concurrent connections")
	flag.IntVar(&messages, "messages", 1000, "messages per connection")
	flag.IntVar(&payloadSize, "payload-size", 64, "payload bytes")
	flag.DurationVar(&timeout, "timeout", 30*time.Second, "overall timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := benchclient.Run(ctx, benchclient.Config{
		Addr:            addr,
		Protocol:        benchclient.Protocol(protocol),
		Connections:     connections,
		MessagesPerConn: messages,
		PayloadSize:     payloadSize,
		Timeout:         timeout,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("requests=%d errors=%d elapsed=%s qps=%.2f avg=%s p50=%s p95=%s p99=%s max=%s\n",
		result.TotalRequests,
		result.Errors,
		result.Elapsed,
		result.Throughput,
		result.Avg,
		result.P50,
		result.P95,
		result.P99,
		result.Max,
	)
	if result.Errors != 0 {
		os.Exit(2)
	}
}

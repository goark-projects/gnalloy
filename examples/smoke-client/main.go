package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"goark.dev/gnalloy/examples/internal/smokeclient"
)

func main() {
	var (
		addr     string
		protocol string
		message  string
		count    int
		timeout  time.Duration
	)
	flag.StringVar(&addr, "addr", "127.0.0.1:9000", "server address")
	flag.StringVar(&protocol, "protocol", string(smokeclient.ProtocolRaw), "protocol: raw or length-field")
	flag.StringVar(&message, "message", "ping", "payload")
	flag.IntVar(&count, "count", 1, "round trip count")
	flag.DurationVar(&timeout, "timeout", 3*time.Second, "dial and read timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := smokeclient.Run(ctx, smokeclient.Config{
		Addr:     addr,
		Protocol: smokeclient.Protocol(protocol),
		Message:  []byte(message),
		Count:    count,
		Timeout:  timeout,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("smoke ok addr=%s protocol=%s count=%d\n", addr, protocol, count)
}

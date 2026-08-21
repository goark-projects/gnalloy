package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"goark.dev/gnalloy/examples/internal/stressclient"
)

func main() {
	var (
		addr        string
		protocol    string
		scenario    string
		connections int
		messages    int
		payloadSize int
		timeout     time.Duration
		delay       time.Duration
	)
	flag.StringVar(&addr, "addr", "127.0.0.1:9000", "server address")
	flag.StringVar(&protocol, "protocol", string(stressclient.ProtocolRaw), "protocol: raw or length-field")
	flag.StringVar(&scenario, "scenario", string(stressclient.ScenarioMixed), "scenario: long, short, half-frame, slow or mixed")
	flag.IntVar(&connections, "connections", 64, "concurrent connections")
	flag.IntVar(&messages, "messages", 100, "messages per connection")
	flag.IntVar(&payloadSize, "payload-size", 64, "payload bytes")
	flag.DurationVar(&timeout, "timeout", 30*time.Second, "overall timeout")
	flag.DurationVar(&delay, "delay", time.Millisecond, "delay for half-frame and slow scenarios")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := stressclient.Run(ctx, stressclient.Config{
		Addr:            addr,
		Protocol:        stressclient.Protocol(protocol),
		Scenario:        stressclient.Scenario(scenario),
		Connections:     connections,
		MessagesPerConn: messages,
		PayloadSize:     payloadSize,
		Timeout:         timeout,
		Delay:           delay,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("stress scenario=%s protocol=%s requests=%d errors=%d elapsed=%s\n",
		scenario,
		protocol,
		result.TotalRequests,
		result.Errors,
		result.Elapsed,
	)
	if result.Errors != 0 {
		os.Exit(2)
	}
}

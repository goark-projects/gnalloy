package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"goark.dev/gnalloy/examples/internal/exampleconfig"
	"goark.dev/gnalloy/transport"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("protocol-exchange", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	opts := exampleconfig.Register(fs, "127.0.0.1:9000")
	transportText := fs.String("transport", "tcp", "transport: tcp, udp, raw, l2")
	message := fs.String("message", "ping", "request payload text")
	payloadHex := fs.String("payload-hex", "", "request payload as hex, overrides -message")
	timeout := fs.Duration("timeout", 3*time.Second, "exchange timeout")
	rawProtocol := fs.Int("raw-protocol", 253, "raw IP protocol number")
	rawFamily := fs.String("raw-family", "ipv4", "raw IP family: ipv4 or ipv6")
	rawHeaderIncluded := fs.Bool("raw-header-included", false, "raw socket writes include an IP header")
	etherTypeText := fs.String("l2-ethertype", "0x88b5", "L2 EtherType, 0 means all protocols")
	l2Promiscuous := fs.Bool("l2-promiscuous", false, "enable L2 promiscuous mode when supported")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := opts.Resolve(); err != nil {
		return err
	}
	kind, err := parseTransportKind(*transportText)
	if err != nil {
		return err
	}
	payload, err := requestPayload(*message, *payloadHex)
	if err != nil {
		return err
	}
	config, err := protocolConfig{
		kind:              kind,
		rawProtocol:       *rawProtocol,
		rawFamilyText:     *rawFamily,
		rawHeaderIncluded: *rawHeaderIncluded,
		l2EtherTypeText:   *etherTypeText,
		l2Promiscuous:     *l2Promiscuous,
	}.resolve()
	if err != nil {
		return err
	}

	group, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         opts.Workers,
		PollerConfig: opts.PollerConfig(),
	})
	if err != nil {
		return err
	}
	defer shutdown(group)

	exchanger, err := newExchanger(opts, group, config)
	if err != nil {
		return err
	}
	exchanger.Timeout = *timeout
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	response, err := exchanger.Exchange(ctx, opts.Addr, payload)
	if err != nil {
		return runtimeBoundaryError(kind, err)
	}
	fmt.Fprintf(stdout, "transport=%s response-bytes=%d response=%q\n", kind, len(response), response)
	return nil
}

func shutdown(group *transport.EventLoopGroup) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = group.Shutdown(ctx)
}

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	dnscodec "goark.dev/gnalloy/codec/dns"
	doq "goark.dev/gnalloy/resolver/dns/quic"
	"goark.dev/gnalloy/transport/quic"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("doq-query", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	server := fs.String("server", "dns.google:853", "DNS-over-QUIC server address")
	name := fs.String("name", "example.com", "DNS query name")
	qtypeText := fs.String("type", "A", "DNS query type: A, AAAA, CNAME, MX, NS, PTR, SOA, SRV, TXT")
	timeout := fs.Duration("timeout", 5*time.Second, "query timeout")
	serverName := fs.String("server-name", "", "TLS server name, empty means host from -server")
	insecure := fs.Bool("insecure", false, "skip TLS certificate verification for lab endpoints")
	dryRun := fs.Bool("dry-run", false, "validate configuration without opening a QUIC connection")
	if err := fs.Parse(args); err != nil {
		return err
	}

	qtype, err := parseQueryType(*qtypeText)
	if err != nil {
		return err
	}
	tlsServerName := strings.TrimSpace(*serverName)
	if tlsServerName == "" {
		tlsServerName = hostForSNI(*server)
	}
	tlsConfig := &tls.Config{
		ServerName: tlsServerName,
		MinVersion: tls.VersionTLS13,
	}
	if *insecure {
		// 仅在显式命令行开关开启时跳过证书校验，便于本地 DoQ 实验环境验证。
		tlsConfig.InsecureSkipVerify = true
	}
	if *dryRun {
		fmt.Fprintf(stdout, "dry-run=true server=%s server-name=%s type=%s timeout=%s insecure=%v\n",
			*server, tlsServerName, typeName(qtype), timeout.String(), *insecure)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	reply, err := doq.Exchanger{
		Config:  quic.Config{TLS: tlsConfig},
		Timeout: *timeout,
	}.Exchange(ctx, *server, dnscodec.NewQuery(1, *name, qtype))
	if err != nil {
		return err
	}
	printMessage(stdout, reply)
	return nil
}

func hostForSNI(server string) string {
	server = strings.TrimSpace(server)
	if server == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(server); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(server, "[]")
}

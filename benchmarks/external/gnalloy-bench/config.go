package main

import (
	"flag"
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"

	"goark.dev/gnalloy/transport"
)

const maxInt = int(^uint(0) >> 1)

type config struct {
	Protocol                  string
	Addr                      string
	Payload                   int
	Connections               int
	Messages                  int
	Timeout                   time.Duration
	BackendName               string
	Backend                   transport.BackendKind
	Boss                      int
	Workers                   int
	ReadBufferSize            int
	ReusePort                 bool
	Mmap                      bool
	MmapBlockSize             int
	MmapBlocks                int
	IOUringFixedBuffers       bool
	IOUringMultishotAccept    bool
	IOUringSQPoll             bool
	LatencySampleRate         int
	WarmupMessages            int
	ALPN                      string
	TLSVersion                string
	CipherSuites              string
	CipherSuiteIDs            []uint16
	AllowInsecureCipherSuites bool
	HTTP1Mode                 string
}

func parseConfig(args []string) (config, error) {
	cfg := config{
		Protocol:       "tcp-echo",
		Addr:           "127.0.0.1:0",
		Payload:        1024,
		Connections:    256,
		Messages:       100000,
		Timeout:        5 * time.Minute,
		BackendName:    "default",
		Boss:           1,
		Workers:        0,
		ReadBufferSize: 0,
		MmapBlockSize:  4096,
		MmapBlocks:     4096,
		ALPN:           "http/1.1",
		TLSVersion:     defaultTLSVersion,
		HTTP1Mode:      defaultHTTP1Mode,
	}
	fs := flag.NewFlagSet("gnalloy-bench", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.Protocol, "protocol", cfg.Protocol, "benchmark protocol")
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.IntVar(&cfg.Payload, "payload", cfg.Payload, "payload bytes")
	fs.IntVar(&cfg.Connections, "connections", cfg.Connections, "concurrent connections")
	fs.IntVar(&cfg.Messages, "messages", cfg.Messages, "messages per connection")
	fs.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "overall timeout")
	fs.StringVar(&cfg.BackendName, "backend", cfg.BackendName, "poller backend: default, std, epoll, iouring, kqueue, iocp, memory")
	fs.IntVar(&cfg.Boss, "boss", cfg.Boss, "boss event-loop count")
	fs.IntVar(&cfg.Workers, "workers", cfg.Workers, "worker event-loop count; 0 selects a backend-aware default")
	fs.IntVar(&cfg.ReadBufferSize, "read-buffer-size", cfg.ReadBufferSize, "per-read ByteBuf size; 0 selects max(payload, 4096)")
	fs.BoolVar(&cfg.ReusePort, "reuseport", cfg.ReusePort, "enable SO_REUSEPORT when the platform supports it")
	fs.BoolVar(&cfg.Mmap, "mmap", cfg.Mmap, "use one mmap allocator per worker event loop")
	fs.IntVar(&cfg.MmapBlockSize, "mmap-block-size", cfg.MmapBlockSize, "mmap allocator block size")
	fs.IntVar(&cfg.MmapBlocks, "mmap-blocks", cfg.MmapBlocks, "mmap allocator block count per worker")
	fs.BoolVar(&cfg.IOUringFixedBuffers, "iouring-fixed-buffers", cfg.IOUringFixedBuffers, "register mmap allocator blocks as io_uring fixed buffers")
	fs.BoolVar(&cfg.IOUringMultishotAccept, "iouring-multishot-accept", cfg.IOUringMultishotAccept, "enable io_uring multishot accept")
	fs.BoolVar(&cfg.IOUringSQPoll, "iouring-sqpoll", cfg.IOUringSQPoll, "enable io_uring SQPOLL")
	fs.IntVar(&cfg.LatencySampleRate, "latency-sample-rate", cfg.LatencySampleRate, "record one round-trip latency sample every N messages per connection; 0 disables latency sampling")
	fs.IntVar(&cfg.WarmupMessages, "warmup-messages", cfg.WarmupMessages, "messages per connection sent before timed measurement; 0 disables in-process warmup")
	fs.StringVar(&cfg.ALPN, "alpn", cfg.ALPN, "comma-separated TLS ALPN protocols for HTTPS protocols")
	fs.StringVar(&cfg.TLSVersion, "tls-version", cfg.TLSVersion, "TLS protocol version: 1.1, 1.2 or 1.3")
	fs.StringVar(&cfg.CipherSuites, "cipher-suites", cfg.CipherSuites, "comma-separated TLS cipher suites using IANA/Java, OpenSSL or hexadecimal names")
	fs.BoolVar(&cfg.AllowInsecureCipherSuites, "allow-insecure-cipher-suites", cfg.AllowInsecureCipherSuites, "allow legacy cipher suites flagged insecure by the Go runtime")
	fs.StringVar(&cfg.HTTP1Mode, "http1-mode", cfg.HTTP1Mode, "HTTP/1 server mode: codec or raw")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	return cfg, cfg.resolve()
}

func (c *config) resolve() error {
	if c == nil {
		return errInvalidConfig
	}
	backend, err := parseBackend(c.BackendName)
	if err != nil {
		return err
	}
	c.Backend = backend
	if c.Protocol == "https2" && c.ALPN == "http/1.1" {
		c.ALPN = "h2"
	}
	if c.Protocol == "http3" && c.ALPN == "http/1.1" {
		c.ALPN = http3ALPNValue
	}
	tlsVersion, err := normalizeTLSVersion(c.TLSVersion)
	if err != nil {
		return err
	}
	c.TLSVersion = tlsVersion
	if err := c.resolveCipherSuites(); err != nil {
		return err
	}
	http1Mode, err := normalizeHTTP1Mode(c.HTTP1Mode)
	if err != nil {
		return err
	}
	c.HTTP1Mode = http1Mode
	if c.Workers == 0 {
		c.Workers = defaultWorkerCount(workerSizingInput{
			GOOS:       runtime.GOOS,
			Backend:    c.Backend,
			GOMAXPROCS: runtime.GOMAXPROCS(0),
		})
	}
	if c.ReadBufferSize == 0 {
		c.ReadBufferSize = defaultReadBufferSize(c.Payload)
	}
	return c.validate()
}

func (c config) validate() error {
	switch strings.TrimSpace(c.Protocol) {
	case "tcp-echo", "udp-echo", "http1", "https1", "http2", "https2", "http3":
	default:
		return fmt.Errorf("%w: %s", errUnsupportedProtocol, c.Protocol)
	}
	if strings.TrimSpace(c.Addr) == "" {
		return fmt.Errorf("%w: empty addr", errInvalidConfig)
	}
	if c.Payload <= 0 || c.Connections <= 0 || c.Messages <= 0 {
		return fmt.Errorf("%w: payload, connections and messages must be positive", errInvalidConfig)
	}
	if c.Connections > maxInt/c.Messages {
		return fmt.Errorf("%w: connections * messages overflows int", errInvalidConfig)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("%w: timeout must be positive", errInvalidConfig)
	}
	if c.Boss <= 0 || c.Workers <= 0 {
		return fmt.Errorf("%w: boss and workers must be positive", errInvalidConfig)
	}
	if c.ReadBufferSize <= 0 {
		return fmt.Errorf("%w: read-buffer-size must be positive", errInvalidConfig)
	}
	if c.Mmap && (c.MmapBlockSize <= 0 || c.MmapBlocks <= 0) {
		return fmt.Errorf("%w: mmap block size and blocks must be positive", errInvalidConfig)
	}
	if c.Mmap && c.MmapBlockSize > maxInt/c.MmapBlocks {
		return fmt.Errorf("%w: mmap block size and blocks overflow", errInvalidConfig)
	}
	if c.Mmap && c.ReadBufferSize > c.MmapBlockSize {
		return fmt.Errorf("%w: read-buffer-size must fit mmap block size", errInvalidConfig)
	}
	if c.IOUringFixedBuffers && (!c.Mmap || c.Backend != transport.BackendIOUring) {
		return fmt.Errorf("%w: fixed buffers require iouring backend with mmap", errInvalidConfig)
	}
	if (c.IOUringMultishotAccept || c.IOUringSQPoll) && c.Backend != transport.BackendIOUring {
		return fmt.Errorf("%w: io_uring options require iouring backend", errInvalidConfig)
	}
	if c.LatencySampleRate < 0 {
		return fmt.Errorf("%w: latency-sample-rate must not be negative", errInvalidConfig)
	}
	if c.WarmupMessages < 0 {
		return fmt.Errorf("%w: warmup-messages must not be negative", errInvalidConfig)
	}
	if c.Protocol == "https2" && c.TLSVersion == tlsVersion11 {
		return fmt.Errorf("%w: HTTP/2 over TLS requires TLS 1.2 or newer", errInvalidConfig)
	}
	if c.HTTP1Mode != defaultHTTP1Mode && c.Protocol != "http1" && c.Protocol != "https1" {
		return fmt.Errorf("%w: http1-mode requires HTTP/1 protocol", errInvalidConfig)
	}
	if c.Protocol == "http3" {
		return ensureHTTP3Config(c)
	}
	return nil
}

func (c *config) resolveCipherSuites() error {
	if strings.TrimSpace(c.CipherSuites) == "" {
		c.CipherSuites = ""
		c.CipherSuiteIDs = nil
		return nil
	}
	ids, err := parseCipherSuiteList(c.CipherSuites, c.AllowInsecureCipherSuites)
	if err != nil {
		return err
	}
	if c.TLSVersion == tlsVersion13 {
		return fmt.Errorf("%w: cipher-suites are configurable only for TLS 1.1 and TLS 1.2", errInvalidConfig)
	}
	c.CipherSuiteIDs = ids
	c.CipherSuites = strings.Join(cipherSuiteNames(ids), ",")
	return nil
}

func parseBackend(name string) (transport.BackendKind, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "default":
		return transport.DefaultBackend(), nil
	case "memory":
		return transport.BackendMemory, nil
	case "std":
		return transport.BackendStd, nil
	case "epoll":
		return transport.BackendEpoll, nil
	case "iouring", "io_uring":
		return transport.BackendIOUring, nil
	case "kqueue":
		return transport.BackendKqueue, nil
	case "iocp":
		return transport.BackendIOCP, nil
	default:
		return 0, fmt.Errorf("%w: %s", errInvalidBackend, name)
	}
}

func backendLabel(backend transport.BackendKind) string {
	switch backend {
	case transport.BackendMemory:
		return "memory"
	case transport.BackendStd:
		return "std"
	case transport.BackendEpoll:
		return "epoll"
	case transport.BackendKqueue:
		return "kqueue"
	case transport.BackendIOUring:
		return "iouring"
	case transport.BackendIOCP:
		return "iocp"
	default:
		return "unknown"
	}
}

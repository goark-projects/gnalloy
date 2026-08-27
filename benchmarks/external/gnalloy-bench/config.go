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
	Protocol       string
	Addr           string
	Payload        int
	Connections    int
	Messages       int
	Timeout        time.Duration
	BackendName    string
	Backend        transport.BackendKind
	Boss           int
	Workers        int
	ReadBufferSize int
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
		ReadBufferSize: 4096,
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
	fs.IntVar(&cfg.ReadBufferSize, "read-buffer-size", cfg.ReadBufferSize, "per-read ByteBuf size")
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
	if c.Workers == 0 {
		c.Workers = defaultWorkerCount(workerSizingInput{
			GOOS:       runtime.GOOS,
			Backend:    c.Backend,
			GOMAXPROCS: runtime.GOMAXPROCS(0),
		})
	}
	return c.validate()
}

func (c config) validate() error {
	if strings.TrimSpace(c.Protocol) != "tcp-echo" {
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

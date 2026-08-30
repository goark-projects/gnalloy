package main

import (
	"errors"
	"runtime"
	"strconv"
	"testing"
	"time"

	"goark.dev/gnalloy/transport"
)

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-protocol", "tcp-echo",
		"-addr", "127.0.0.1:0",
		"-payload", "32",
		"-connections", "2",
		"-messages", "3",
		"-timeout", "2s",
		"-backend", "std",
		"-boss", "1",
		"-workers", "2",
		"-read-buffer-size", "8192",
		"-reuseport",
		"-warmup-messages", "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Payload != 32 || cfg.Connections != 2 || cfg.Messages != 3 || cfg.Timeout != 2*time.Second {
		t.Fatalf("cfg=%+v", cfg)
	}
	if cfg.Boss != 1 || cfg.Workers != 2 || cfg.ReadBufferSize != 8192 || backendLabel(cfg.Backend) != "std" {
		t.Fatalf("cfg=%+v", cfg)
	}
	if !cfg.ReusePort {
		t.Fatalf("reuseport=%v, want true", cfg.ReusePort)
	}
	if cfg.WarmupMessages != 7 {
		t.Fatalf("warmupMessages=%d, want 7", cfg.WarmupMessages)
	}
	if cfg.ALPN != "http/1.1" {
		t.Fatalf("alpn=%q, want http/1.1", cfg.ALPN)
	}
}

func TestParseConfigSupportsHTTPS1ALPN(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-protocol", "https1",
		"-alpn", "h2,http/1.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Protocol != "https1" || cfg.ALPN != "h2,http/1.1" {
		t.Fatalf("cfg=%+v", cfg)
	}
}

func TestParseConfigSupportsHTTP1RawMode(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-protocol", "http1",
		"-http1-mode", "raw",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP1Mode != http1ModeRaw {
		t.Fatalf("http1Mode=%q, want raw", cfg.HTTP1Mode)
	}
}

func TestParseConfigRejectsHTTP1ModeForHTTP2(t *testing.T) {
	_, err := parseConfig([]string{
		"-protocol", "http2",
		"-http1-mode", "raw",
	})
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, errInvalidConfig)
	}
}

func TestParseConfigSupportsTLSVersions(t *testing.T) {
	for _, version := range []string{"1.1", "1.2", "1.3"} {
		cfg, err := parseConfig([]string{
			"-protocol", "https1",
			"-tls-version", version,
		})
		if err != nil {
			t.Fatalf("version %s: %v", version, err)
		}
		if cfg.TLSVersion != version {
			t.Fatalf("version=%q, want %s", cfg.TLSVersion, version)
		}
	}
}

func TestParseConfigRejectsHTTP2TLS11(t *testing.T) {
	_, err := parseConfig([]string{"-protocol", "https2", "-tls-version", "1.1"})
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, errInvalidConfig)
	}
}

func TestParseConfigSupportsHTTP2Family(t *testing.T) {
	cfg, err := parseConfig([]string{"-protocol", "http2"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Protocol != "http2" {
		t.Fatalf("protocol=%q, want http2", cfg.Protocol)
	}

	cfg, err = parseConfig([]string{"-protocol", "https2"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Protocol != "https2" || cfg.ALPN != "h2" {
		t.Fatalf("cfg=%+v, want https2 with h2 ALPN", cfg)
	}
}

func TestParseConfigSupportsHTTP3(t *testing.T) {
	cfg, err := parseConfig([]string{"-protocol", "http3"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Protocol != "http3" || cfg.ALPN != "h3" || cfg.TLSVersion != "1.3" {
		t.Fatalf("cfg=%+v, want http3 with h3/TLS1.3", cfg)
	}
}

func TestParseConfigRejectsHTTP3TLS12(t *testing.T) {
	_, err := parseConfig([]string{"-protocol", "http3", "-tls-version", "1.2"})
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, errInvalidConfig)
	}
}

func TestParseConfigSupportsUDPEcho(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-protocol", "udp-echo",
		"-payload", "128",
		"-connections", "2",
		"-messages", "3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Protocol != "udp-echo" || cfg.Payload != 128 || cfg.Connections != 2 || cfg.Messages != 3 {
		t.Fatalf("cfg=%+v", cfg)
	}
}

func TestParseConfigResolvesNativePerformanceFlags(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-backend", "iouring",
		"-mmap",
		"-mmap-block-size", "8192",
		"-mmap-blocks", "1024",
		"-iouring-fixed-buffers",
		"-iouring-multishot-accept",
		"-iouring-sqpoll",
		"-latency-sample-rate", "32",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backend != transport.BackendIOUring || !cfg.Mmap || cfg.MmapBlockSize != 8192 || cfg.MmapBlocks != 1024 {
		t.Fatalf("cfg=%+v", cfg)
	}
	if !cfg.IOUringFixedBuffers || !cfg.IOUringMultishotAccept || !cfg.IOUringSQPoll {
		t.Fatalf("iouring flags=%+v", cfg)
	}
	if cfg.LatencySampleRate != 32 {
		t.Fatalf("latencySampleRate=%d, want 32", cfg.LatencySampleRate)
	}
}

func TestParseConfigResolvesAutoWorkers(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-backend", "iocp",
		"-workers", "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := defaultWorkerCount(workerSizingInput{
		GOOS:       runtime.GOOS,
		Backend:    transport.BackendIOCP,
		GOMAXPROCS: runtime.GOMAXPROCS(0),
	})
	if cfg.Workers != want {
		t.Fatalf("workers=%d, want %d", cfg.Workers, want)
	}
}

func TestParseConfigResolvesAutoReadBufferSize(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-payload", "16384",
		"-read-buffer-size", "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReadBufferSize != 16384 {
		t.Fatalf("readBufferSize=%d, want 16384", cfg.ReadBufferSize)
	}
}

func TestParseConfigKeepsMinimumAutoReadBufferSize(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-payload", "64",
		"-read-buffer-size", "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReadBufferSize != 4096 {
		t.Fatalf("readBufferSize=%d, want 4096", cfg.ReadBufferSize)
	}
}

func TestDefaultWorkerCountCapsWindowsIOCP(t *testing.T) {
	got := defaultWorkerCount(workerSizingInput{
		GOOS:       "windows",
		Backend:    transport.BackendIOCP,
		GOMAXPROCS: 16,
	})
	if got != 8 {
		t.Fatalf("workers=%d, want 8", got)
	}
}

func TestDefaultWorkerCountCapsLinuxEpoll(t *testing.T) {
	got := defaultWorkerCount(workerSizingInput{
		GOOS:       "linux",
		Backend:    transport.BackendEpoll,
		GOMAXPROCS: 8,
	})
	if got != 4 {
		t.Fatalf("workers=%d, want 4", got)
	}
}

func TestDefaultWorkerCountCapsLinuxIOUring(t *testing.T) {
	got := defaultWorkerCount(workerSizingInput{
		GOOS:       "linux",
		Backend:    transport.BackendIOUring,
		GOMAXPROCS: 8,
	})
	if got != 4 {
		t.Fatalf("workers=%d, want 4", got)
	}
}

func TestDefaultWorkerCountKeepsNonIOCPParallelism(t *testing.T) {
	got := defaultWorkerCount(workerSizingInput{
		GOOS:       "linux",
		Backend:    transport.BackendStd,
		GOMAXPROCS: 16,
	})
	if got != 16 {
		t.Fatalf("workers=%d, want 16", got)
	}
}

func TestDefaultWorkerCountNormalizesInvalidCPUCount(t *testing.T) {
	got := defaultWorkerCount(workerSizingInput{
		GOOS:       "windows",
		Backend:    transport.BackendIOCP,
		GOMAXPROCS: 0,
	})
	if got != 1 {
		t.Fatalf("workers=%d, want 1", got)
	}
}

func TestParseConfigRejectsUnsupportedProtocol(t *testing.T) {
	_, err := parseConfig([]string{"-protocol", "sctp-echo"})
	if !errors.Is(err, errUnsupportedProtocol) {
		t.Fatalf("err=%v, want %v", err, errUnsupportedProtocol)
	}
}

func TestParseConfigRejectsNegativeWorkers(t *testing.T) {
	_, err := parseConfig([]string{"-workers", "-1"})
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, errInvalidConfig)
	}
}

func TestParseConfigRejectsNegativeReadBufferSize(t *testing.T) {
	_, err := parseConfig([]string{"-read-buffer-size", "-1"})
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, errInvalidConfig)
	}
}

func TestParseConfigRejectsNegativeLatencySampleRate(t *testing.T) {
	_, err := parseConfig([]string{"-latency-sample-rate", "-1"})
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, errInvalidConfig)
	}
}

func TestParseConfigRejectsNegativeWarmupMessages(t *testing.T) {
	_, err := parseConfig([]string{"-warmup-messages", "-1"})
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, errInvalidConfig)
	}
}

func TestParseConfigRejectsInvalidTLSVersion(t *testing.T) {
	_, err := parseConfig([]string{"-protocol", "https1", "-tls-version", "1.0"})
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, errInvalidConfig)
	}
}

func TestParseConfigRejectsFixedBuffersWithoutMmap(t *testing.T) {
	_, err := parseConfig([]string{"-backend", "iouring", "-iouring-fixed-buffers"})
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, errInvalidConfig)
	}
}

func TestParseConfigRejectsMmapBlockSmallerThanReadBuffer(t *testing.T) {
	_, err := parseConfig([]string{"-mmap", "-mmap-block-size", "1024", "-read-buffer-size", "4096"})
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, errInvalidConfig)
	}
}

func TestParseConfigRejectsMmapSizeOverflow(t *testing.T) {
	_, err := parseConfig([]string{"-mmap", "-mmap-block-size", strconv.Itoa(maxInt), "-mmap-blocks", "2"})
	if !errors.Is(err, errInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, errInvalidConfig)
	}
}

func TestParseConfigRejectsInvalidBackend(t *testing.T) {
	_, err := parseConfig([]string{"-backend", "select"})
	if !errors.Is(err, errInvalidBackend) {
		t.Fatalf("err=%v, want %v", err, errInvalidBackend)
	}
}

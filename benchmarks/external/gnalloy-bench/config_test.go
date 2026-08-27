package main

import (
	"errors"
	"runtime"
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
	_, err := parseConfig([]string{"-protocol", "udp-echo"})
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

func TestParseConfigRejectsInvalidBackend(t *testing.T) {
	_, err := parseConfig([]string{"-backend", "select"})
	if !errors.Is(err, errInvalidBackend) {
		t.Fatalf("err=%v, want %v", err, errInvalidBackend)
	}
}

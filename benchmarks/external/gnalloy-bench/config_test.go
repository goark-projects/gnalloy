package main

import (
	"errors"
	"testing"
	"time"
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

func TestParseConfigRejectsUnsupportedProtocol(t *testing.T) {
	_, err := parseConfig([]string{"-protocol", "udp-echo"})
	if !errors.Is(err, errUnsupportedProtocol) {
		t.Fatalf("err=%v, want %v", err, errUnsupportedProtocol)
	}
}

func TestParseConfigRejectsInvalidBackend(t *testing.T) {
	_, err := parseConfig([]string{"-backend", "select"})
	if !errors.Is(err, errInvalidBackend) {
		t.Fatalf("err=%v, want %v", err, errInvalidBackend)
	}
}

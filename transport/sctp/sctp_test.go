package sctp

import (
	"errors"
	"runtime"
	"testing"

	"goark.dev/gnalloy/transport"
)

func TestNormalizeConfigDefaults(t *testing.T) {
	cfg := normalizeConfig(Config{})
	if cfg.Backlog <= 0 || cfg.ReadBufferSize <= 0 || cfg.ConnectTimeoutMillis <= 0 {
		t.Fatalf("cfg=%+v, want positive defaults", cfg)
	}
	if cfg.OutboundStreams != defaultOutboundStreams || cfg.InboundStreams != defaultInboundStreams {
		t.Fatalf("streams=%d/%d, want defaults", cfg.OutboundStreams, cfg.InboundStreams)
	}
	if cfg.WriteBufferWatermark.High <= cfg.WriteBufferWatermark.Low {
		t.Fatalf("watermark=%+v, want normalized", cfg.WriteBufferWatermark)
	}
}

func TestParseAddressRequiresIPHostPort(t *testing.T) {
	addr, err := parseAddress("127.0.0.1:9000")
	if err != nil {
		t.Fatal(err)
	}
	if addr.port != 9000 || addr.ip == nil || addr.ipv6 {
		t.Fatalf("addr=%+v", addr)
	}
	if _, err := parseAddress("example.com:9000"); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("err=%v, want invalid address", err)
	}
}

func TestUnsupportedBackendIsExplicit(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("linux uses native SCTP socket when the kernel supports it")
	}
	_, err := listenSCTP("127.0.0.1:0", normalizeConfig(Config{}).socketOptions())
	if !errors.Is(err, ErrUnsupportedSCTP) {
		t.Fatalf("err=%v, want %v", err, ErrUnsupportedSCTP)
	}
	if _, err := dialSCTP("127.0.0.1:9", normalizeConfig(Config{}).socketOptions()); !errors.Is(err, ErrUnsupportedSCTP) {
		t.Fatalf("err=%v, want %v", err, ErrUnsupportedSCTP)
	}
}

func TestSocketAddressConversion(t *testing.T) {
	addr, err := parseAddress("[::1]:9000")
	if err != nil {
		t.Fatal(err)
	}
	sa := addr.socketAddress()
	if !sa.Valid() || sa.Family != transport.SocketFamilyIPv6 || sa.Port != 9000 {
		t.Fatalf("socket address=%+v", sa)
	}
}

package sctp

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
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

func TestValidateRuntimeRejectsCompletionClientBeforeDial(t *testing.T) {
	group := newSCTPTestGroup(t, transport.BackendMemory)
	err := ValidateRuntime(RuntimeCheck{
		Role:    EndpointRoleClient,
		Address: "127.0.0.1:9",
		Group:   group,
		Config:  Config{},
	})
	if !errors.Is(err, ErrUnsupportedCompletion) {
		t.Fatalf("err=%v, want %v", err, ErrUnsupportedCompletion)
	}

	_, err = NewTransport(Config{}).Dial(context.Background(), bootstrap.ClientConfig{
		Address:     "127.0.0.1:9",
		Group:       group,
		Initializer: func(channel.Channel) error { return nil },
	})
	if !errors.Is(err, ErrUnsupportedCompletion) {
		t.Fatalf("dial err=%v, want %v", err, ErrUnsupportedCompletion)
	}
}

func TestValidateRuntimeRejectsCompletionWorker(t *testing.T) {
	boss := newSCTPTestGroup(t, transport.BackendStd)
	worker := newSCTPTestGroup(t, transport.BackendMemory)
	err := ValidateRuntime(RuntimeCheck{
		Role:        EndpointRoleServer,
		Address:     "127.0.0.1:0",
		BossGroup:   boss,
		WorkerGroup: worker,
		Config:      Config{},
	})
	if !errors.Is(err, ErrUnsupportedCompletion) {
		t.Fatalf("err=%v, want %v", err, ErrUnsupportedCompletion)
	}
}

func TestValidateRuntimeRejectsClientPortZero(t *testing.T) {
	group := newSCTPTestGroup(t, transport.BackendStd)
	err := ValidateRuntime(RuntimeCheck{
		Role:    EndpointRoleClient,
		Address: "127.0.0.1:0",
		Group:   group,
		Config:  Config{},
	})
	if !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidAddress)
	}
}

func TestValidateRuntimeRejectsUnsupportedPlatform(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("linux provides the native SCTP socket entry")
	}
	group := newSCTPTestGroup(t, transport.BackendStd)
	err := ValidateRuntime(RuntimeCheck{
		Role:    EndpointRoleClient,
		Address: "127.0.0.1:9",
		Group:   group,
		Config:  Config{},
	})
	if !errors.Is(err, ErrUnsupportedSCTP) {
		t.Fatalf("err=%v, want %v", err, ErrUnsupportedSCTP)
	}
}

func TestValidateConfigRejectsNegativeSocketBuffers(t *testing.T) {
	if err := ValidateConfig(Config{SendBufferSize: -1}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("send buffer err=%v, want %v", err, ErrInvalidConfig)
	}
	if err := ValidateConfig(Config{ReceiveBufferSize: -1}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("receive buffer err=%v, want %v", err, ErrInvalidConfig)
	}
}

func TestDetectRuntimeSupportReportsStableBoundary(t *testing.T) {
	support := DetectRuntimeSupport()
	if support.Platform != runtime.GOOS {
		t.Fatalf("platform=%q, want %q", support.Platform, runtime.GOOS)
	}
	if support.CompletionPoller {
		t.Fatalf("support=%+v, SCTP must not claim completion poller support", support)
	}
}

func newSCTPTestGroup(t *testing.T, backend transport.BackendKind) *transport.EventLoopGroup {
	t.Helper()
	group, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         1,
		PollerConfig: transport.Config{Backend: backend},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = group.Shutdown(ctx)
	})
	return group
}

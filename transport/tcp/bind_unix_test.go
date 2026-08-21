//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package tcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

var errRegisterFailed = errors.New("register failed")

type bindFailurePoller struct {
	failRegister bool
	addr         *string
}

func (p *bindFailurePoller) Model() transport.PollerModel {
	return transport.PollerReadiness
}

func (p *bindFailurePoller) Backend() transport.BackendKind {
	return transport.BackendMemory
}

func (p *bindFailurePoller) Register(fd transport.FDRef, ch transport.ChannelID, _ transport.ReadyMask) error {
	if p.failRegister {
		return errRegisterFailed
	}
	if p.addr != nil && ch != 0 {
		*p.addr = socketName(fd.FD, "")
	}
	return nil
}

func (p *bindFailurePoller) Modify(transport.FDRef, transport.ReadyMask) error {
	return nil
}

func (p *bindFailurePoller) Deregister(transport.FDRef) error {
	return nil
}

func (p *bindFailurePoller) Submit(req transport.IORequest) error {
	if req.Op == transport.OpWakeup {
		return nil
	}
	return transport.ErrInvalidIORequest
}

func (p *bindFailurePoller) Poll([]transport.PollEvent, int) (int, error) {
	time.Sleep(time.Millisecond)
	return 0, nil
}

func (p *bindFailurePoller) Wakeup() error {
	return nil
}

func (p *bindFailurePoller) Close() error {
	return nil
}

func TestBindFailureClosesPartiallyRegisteredReusePortListeners(t *testing.T) {
	var firstAddr string
	boss, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size: 2,
		PollerFactory: func(index int) (transport.Poller, error) {
			return &bindFailurePoller{failRegister: index == 1, addr: &firstAddr}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	workers, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         1,
		PollerConfig: transport.Config{Backend: transport.BackendMemory},
	})
	if err != nil {
		_ = boss.Close()
		t.Fatal(err)
	}
	defer shutdownTestGroup(t, boss)
	defer shutdownTestGroup(t, workers)

	cfg := DefaultConfig()
	cfg.ReusePort = true
	_, err = bootstrap.NewServerBootstrap().
		Group(boss, workers).
		Transport(NewTransport(cfg)).
		ChildHandler(func(channel.Channel) {}).
		BindContext(context.Background(), "127.0.0.1:0")
	if !errors.Is(err, errRegisterFailed) {
		t.Fatalf("bind err=%v, want %v", err, errRegisterFailed)
	}
	if firstAddr == "" {
		t.Fatal("first listener address was not captured")
	}

	ls, err := listenTCP(firstAddr, DefaultConfig().socketOptions())
	if err != nil {
		t.Fatalf("rebind after failed bind: %v", err)
	}
	_ = closeFD(ls.fd)
}

func shutdownTestGroup(t *testing.T, group *transport.EventLoopGroup) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := group.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

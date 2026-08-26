package raw

import (
	"context"
	"errors"
	"testing"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/transport"
)

func TestTransportImplementsBootstrapDialer(t *testing.T) {
	var _ bootstrap.ClientTransport = (*Transport)(nil)
}

func TestRawDialerRejectsInvalidRemoteBeforeSocket(t *testing.T) {
	group, err := transport.NewEventLoopGroup(transport.EventLoopGroupConfig{
		Size:         1,
		PollerConfig: transport.Config{Backend: transport.BackendMemory},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer group.Close()

	_, err = bootstrap.NewDialer().
		Group(group).
		Transport(NewTransport(DefaultConfig())).
		DialContext(context.Background(), "not-an-ip")
	if !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidAddress)
	}
}

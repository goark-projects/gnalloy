//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package unix

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/transport"
)

func TestUnsupportedAdvancedUnixFeatures(t *testing.T) {
	if _, err := ListenDatagram("unix://advanced.sock", DefaultConfig()); !errors.Is(err, ErrUnsupportedDatagramSocket) {
		t.Fatalf("ListenDatagram err=%v, want ErrUnsupportedDatagramSocket", err)
	}
	if _, err := PeerCredentials(transport.FDRef{FD: -1}); !errors.Is(err, ErrUnsupportedPeerCredentials) {
		t.Fatalf("PeerCredentials err=%v, want ErrUnsupportedPeerCredentials", err)
	}
	if err := SendFD(transport.FDRef{FD: -1}, -1); !errors.Is(err, ErrUnsupportedFDPassing) {
		t.Fatalf("SendFD err=%v, want ErrUnsupportedFDPassing", err)
	}
	if _, err := ReceiveFD(transport.FDRef{FD: -1}); !errors.Is(err, ErrUnsupportedFDPassing) {
		t.Fatalf("ReceiveFD err=%v, want ErrUnsupportedFDPassing", err)
	}
}

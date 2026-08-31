//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package unix

import (
	"path/filepath"
	"testing"
)

func TestDatagramEndpointSendReceive(t *testing.T) {
	left, err := ListenDatagram(filepath.Join(t.TempDir(), "left.sock"), DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer left.Close()
	right, err := ListenDatagram(filepath.Join(t.TempDir(), "right.sock"), DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer right.Close()

	if err := left.SendTo([]byte("ping"), right.Addr().String()); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	n, from, err := right.ReceiveFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "ping" || from.String() != left.Addr().String() {
		t.Fatalf("n=%d payload=%q from=%s", n, buf[:n], from.String())
	}
}

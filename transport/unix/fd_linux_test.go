//go:build linux

package unix

import (
	"os"
	"testing"

	"goark.dev/gnalloy/transport"
	xunix "golang.org/x/sys/unix"
)

func TestPeerCredentialsAndFDPassing(t *testing.T) {
	fds, err := xunix.Socketpair(xunix.AF_UNIX, xunix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer xunix.Close(fds[0])
	defer xunix.Close(fds[1])

	creds, err := PeerCredentials(transport.FDRef{FD: fds[0]})
	if err != nil {
		t.Fatal(err)
	}
	if creds.PID <= 0 || creds.UID != uint32(os.Getuid()) || creds.GID != uint32(os.Getgid()) {
		t.Fatalf("credentials=%+v", creds)
	}

	file, err := os.CreateTemp(t.TempDir(), "fd-pass")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("ok"); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := SendFD(transport.FDRef{FD: fds[0]}, int(file.Fd())); err != nil {
		t.Fatal(err)
	}
	received, err := ReceiveFD(transport.FDRef{FD: fds[1]})
	if err != nil {
		t.Fatal(err)
	}
	defer xunix.Close(received)
	var buf [2]byte
	n, err := xunix.Read(received, buf[:])
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "ok" {
		t.Fatalf("fd payload=%q", buf[:n])
	}
}

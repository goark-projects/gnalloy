//go:build linux

package raw

import (
	"os"
	"strconv"
	"testing"
)

func TestPrivilegedRawSocketOpen(t *testing.T) {
	if os.Getenv("GNALLOY_RAW_PRIVILEGED") != "1" {
		t.Skip("set GNALLOY_RAW_PRIVILEGED=1 to open a privileged raw socket")
	}
	cfg := DefaultConfig()
	if protocolText := os.Getenv("GNALLOY_RAW_PROTOCOL"); protocolText != "" {
		protocol, err := strconv.Atoi(protocolText)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Protocol = protocol
	}
	bindAddr := os.Getenv("GNALLOY_RAW_BIND")
	if bindAddr == "" {
		bindAddr = "0.0.0.0"
	}
	opts, err := cfg.socketOptions()
	if err != nil {
		t.Fatal(err)
	}
	sock, err := listenRaw(bindAddr, opts)
	if err != nil {
		t.Fatalf("open raw socket: %v", err)
	}
	if sock.protocol != cfg.Protocol {
		_ = closeFD(sock.fd)
		t.Fatalf("protocol=%d, want %d", sock.protocol, cfg.Protocol)
	}
	if err := closeFD(sock.fd); err != nil {
		t.Fatal(err)
	}
}

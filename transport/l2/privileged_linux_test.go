//go:build linux

package l2

import (
	"context"
	"encoding/hex"
	"os"
	"strconv"
	"testing"

	"goark.dev/gnalloy/buffer"
)

func TestPrivilegedAFPacketOpen(t *testing.T) {
	iface := os.Getenv("GNALLOY_L2_INTERFACE")
	if iface == "" {
		t.Skip("set GNALLOY_L2_INTERFACE to open a privileged AF_PACKET endpoint")
	}
	cfg := Config{InterfaceName: iface}
	if etherTypeText := os.Getenv("GNALLOY_L2_ETHERTYPE"); etherTypeText != "" {
		etherType, err := strconv.ParseUint(etherTypeText, 0, 16)
		if err != nil {
			t.Fatal(err)
		}
		cfg.EtherType = uint16(etherType)
	}
	ep, err := nativeDriver{}.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open AF_PACKET endpoint: %v", err)
	}
	defer ep.Close()
	if ep.Addr() != iface {
		t.Fatalf("addr=%q, want %q", ep.Addr(), iface)
	}
	if frameHex := os.Getenv("GNALLOY_L2_FRAME_HEX"); frameHex != "" {
		data, err := hex.DecodeString(frameHex)
		if err != nil {
			t.Fatal(err)
		}
		alloc := buffer.NewHeapAllocator()
		defer alloc.Close()
		payload, err := alloc.Acquire(len(data))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := payload.WriteBytes(data); err != nil {
			payload.Release()
			t.Fatal(err)
		}
		if err := ep.WriteFrame(context.Background(), Frame{Payload: payload}); err != nil {
			payload.Release()
			t.Fatal(err)
		}
		payload.Release()
	}
}

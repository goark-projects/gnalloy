//go:build windows

package npcap

import (
	"context"
	"encoding/hex"
	"os"
	"strconv"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport/l2"
)

func TestPrivilegedNpcapOpen(t *testing.T) {
	iface := os.Getenv("GNALLOY_NPCAP_INTERFACE")
	if iface == "" {
		t.Skip("set GNALLOY_NPCAP_INTERFACE to open a native Npcap endpoint")
	}
	cfg := l2.Config{InterfaceName: iface}
	if etherTypeText := os.Getenv("GNALLOY_NPCAP_ETHERTYPE"); etherTypeText != "" {
		etherType, err := strconv.ParseUint(etherTypeText, 0, 16)
		if err != nil {
			t.Fatal(err)
		}
		cfg.EtherType = uint16(etherType)
	}
	ep, err := NewDriver(nil, Config{Immediate: true}).Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open Npcap endpoint: %v", err)
	}
	defer ep.Close()
	if ep.Addr() == "" {
		t.Fatal("empty endpoint address")
	}
	if frameHex := os.Getenv("GNALLOY_NPCAP_FRAME_HEX"); frameHex != "" {
		writeTestFrame(t, ep, frameHex)
	}
}

func writeTestFrame(t *testing.T, ep l2.Endpoint, frameHex string) {
	t.Helper()
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
	if err := ep.WriteFrame(context.Background(), l2.Frame{Payload: payload}); err != nil {
		payload.Release()
		t.Fatal(err)
	}
	payload.Release()
}

package main

import (
	"bytes"
	"errors"
	"testing"

	"goark.dev/gnalloy/transport/raw"
)

func TestParseTransportKindAcceptsAliases(t *testing.T) {
	tests := map[string]transportKind{
		"tcp":      transportTCP,
		"stream":   transportTCP,
		"udp":      transportUDP,
		"datagram": transportUDP,
		"raw":      transportRaw,
		"packet":   transportRaw,
		"l2":       transportL2,
		"frame":    transportL2,
	}
	for input, want := range tests {
		got, err := parseTransportKind(input)
		if err != nil {
			t.Fatalf("parse %q: %v", input, err)
		}
		if got != want {
			t.Fatalf("parse %q=%q, want %q", input, got, want)
		}
	}
}

func TestParseTransportKindRejectsUnknown(t *testing.T) {
	_, err := parseTransportKind("sctp")
	if !errors.Is(err, errInvalidTransport) {
		t.Fatalf("err=%v, want %v", err, errInvalidTransport)
	}
}

func TestParseRawFamily(t *testing.T) {
	family, err := parseRawFamily("ip6")
	if err != nil {
		t.Fatal(err)
	}
	if family != raw.FamilyIPv6 {
		t.Fatalf("family=%v, want IPv6", family)
	}
}

func TestParseEtherTypeAcceptsDecimalAndHex(t *testing.T) {
	for _, input := range []string{"34997", "0x88b5"} {
		got, err := parseEtherType(input)
		if err != nil {
			t.Fatalf("parse %q: %v", input, err)
		}
		if got != 0x88b5 {
			t.Fatalf("parse %q=%#x, want 0x88b5", input, got)
		}
	}
}

func TestRequestPayloadPrefersHex(t *testing.T) {
	payload, err := requestPayload("ignored", "70696e67")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, []byte("ping")) {
		t.Fatalf("payload=%q, want ping", payload)
	}
}

func TestProtocolConfigResolveValidatesRawAndL2(t *testing.T) {
	rawCfg, err := (protocolConfig{
		kind:          transportRaw,
		rawProtocol:   253,
		rawFamilyText: "ipv4",
	}).resolve()
	if err != nil {
		t.Fatal(err)
	}
	if rawCfg.rawFamily != raw.FamilyIPv4 {
		t.Fatalf("raw family=%v, want IPv4", rawCfg.rawFamily)
	}

	l2Cfg, err := (protocolConfig{
		kind:            transportL2,
		l2EtherTypeText: "0x88b5",
	}).resolve()
	if err != nil {
		t.Fatal(err)
	}
	if l2Cfg.l2EtherTyp != 0x88b5 {
		t.Fatalf("etherType=%#x, want 0x88b5", l2Cfg.l2EtherTyp)
	}
}

func TestRunDryRunDoesNotOpenTransport(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{
		"-dry-run",
		"-transport", "l2",
		"-addr", "eth-test",
		"-payload-hex", "00112233445566778899aabb88b570696e67",
	}, &out)
	if err != nil {
		t.Fatal(err)
	}
	want := "dry-run=true transport=l2 addr=eth-test payload-bytes=18 timeout=3s\n"
	if out.String() != want {
		t.Fatalf("output=%q, want %q", out.String(), want)
	}
}

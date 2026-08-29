package main

import (
	cryptotls "crypto/tls"
	"testing"
)

func TestALPNProtocolsTrimsEmptyItems(t *testing.T) {
	got := alpnProtocols(" h2, http/1.1, ")
	if len(got) != 2 || got[0] != "h2" || got[1] != "http/1.1" {
		t.Fatalf("protocols=%v, want [h2 http/1.1]", got)
	}
}

func TestServerTLSConfigUsesTLS13(t *testing.T) {
	cfg, err := parseConfig([]string{"-protocol", "https1", "-alpn", "http/1.1"})
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig, err := serverTLSConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if tlsConfig.MinVersion != cryptotls.VersionTLS13 || len(tlsConfig.NextProtos) != 1 || tlsConfig.NextProtos[0] != "http/1.1" {
		t.Fatalf("tlsConfig=%+v", tlsConfig)
	}
}

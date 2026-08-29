package macos

import (
	"reflect"
	"testing"
	"time"
)

func TestParseScutilDNSConfigExtractsServersAndSearchDomains(t *testing.T) {
	input := []byte(`DNS configuration

resolver #1
  search domain[0] : lan
  search domain[1] : svc.cluster.local.
  nameserver[0] : 192.168.1.1
  nameserver[1] : 2001:db8::53
  if_index : 12 (en0)
  flags    : Request A records, Request AAAA records

resolver #2
  domain   : corp.example
  nameserver[0] : 10.0.0.53:5353
`)
	cfg, err := ParseScutilDNSConfig(input, ResolverConfig{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	wantServers := []string{"192.168.1.1:53", "[2001:db8::53]:53", "10.0.0.53:5353"}
	if !reflect.DeepEqual(cfg.Servers, wantServers) {
		t.Fatalf("servers=%v, want %v", cfg.Servers, wantServers)
	}
	wantSearch := []string{"lan", "svc.cluster.local", "corp.example"}
	if !reflect.DeepEqual(cfg.SearchDomains, wantSearch) {
		t.Fatalf("search=%v, want %v", cfg.SearchDomains, wantSearch)
	}
	if cfg.Timeout != 2*time.Second || !cfg.TCPFallback {
		t.Fatalf("cfg=%+v, want timeout and tcp fallback", cfg)
	}
}

func TestParseScutilDNSConfigRejectsOutputWithoutNameServers(t *testing.T) {
	_, err := ParseScutilDNSConfig([]byte(`DNS configuration

resolver #1
  domain   : empty.example
`), ResolverConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
}

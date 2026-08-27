package main

import (
	"bytes"
	"errors"
	"net"
	"testing"

	dnscodec "goark.dev/gnalloy/codec/dns"
)

func TestParseQueryTypeAcceptsCommonTypes(t *testing.T) {
	tests := map[string]uint16{
		"a":     dnscodec.TypeA,
		"AAAA":  dnscodec.TypeAAAA,
		"cname": dnscodec.TypeCNAME,
		"mx":    dnscodec.TypeMX,
		"txt":   dnscodec.TypeTXT,
	}
	for input, want := range tests {
		got, err := parseQueryType(input)
		if err != nil {
			t.Fatalf("parse %q: %v", input, err)
		}
		if got != want {
			t.Fatalf("parse %q=%d, want %d", input, got, want)
		}
	}
}

func TestParseQueryTypeRejectsUnknownType(t *testing.T) {
	_, err := parseQueryType("unknown")
	if !errors.Is(err, errInvalidQueryType) {
		t.Fatalf("err=%v, want %v", err, errInvalidQueryType)
	}
}

func TestHostForSNIUsesHostPart(t *testing.T) {
	tests := map[string]string{
		"dns.google:853":             "dns.google",
		"[2001:4860:4860::8888]:853": "2001:4860:4860::8888",
		"cloudflare-dns.com":         "cloudflare-dns.com",
	}
	for input, want := range tests {
		if got := hostForSNI(input); got != want {
			t.Fatalf("hostForSNI(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestPrintMessageFormatsAnswers(t *testing.T) {
	answer, err := dnscodec.NewAResource("example.com", 60, net.IPv4(1, 2, 3, 4))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	printMessage(&out, dnscodec.Message{
		ID:       7,
		Response: true,
		Answers:  []dnscodec.Resource{answer},
	})
	want := "id=7 rcode=0 answers=1 authorities=0 additionals=0\nanswer example.com A ttl=60 1.2.3.4\n"
	if out.String() != want {
		t.Fatalf("output=%q, want %q", out.String(), want)
	}
}

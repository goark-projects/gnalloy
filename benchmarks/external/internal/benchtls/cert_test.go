package benchtls

import (
	"crypto/x509"
	"net"
	"testing"
)

func TestSelfSignedCertificateUsesDefaultServerName(t *testing.T) {
	cert, err := SelfSignedCertificate("")
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != DefaultServerName {
		t.Fatalf("dnsNames=%v, want %s", leaf.DNSNames, DefaultServerName)
	}
}

func TestSelfSignedCertificateSupportsIPName(t *testing.T) {
	cert, err := SelfSignedCertificate("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	want := net.ParseIP("127.0.0.1")
	if len(leaf.IPAddresses) != 1 || !leaf.IPAddresses[0].Equal(want) {
		t.Fatalf("ipAddresses=%v, want %s", leaf.IPAddresses, want)
	}
}

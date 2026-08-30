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
	if leaf.PublicKeyAlgorithm != x509.ECDSA {
		t.Fatalf("publicKeyAlgorithm=%v, want ECDSA", leaf.PublicKeyAlgorithm)
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

func TestSelfSignedCertificateSupportsRSA(t *testing.T) {
	cert, err := SelfSignedCertificateWithAlgorithm(DefaultServerName, CertificateKeyRSA)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if leaf.PublicKeyAlgorithm != x509.RSA {
		t.Fatalf("publicKeyAlgorithm=%v, want RSA", leaf.PublicKeyAlgorithm)
	}
	if leaf.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
		t.Fatalf("keyUsage=%v, want key encipherment", leaf.KeyUsage)
	}
}

func TestSelfSignedCertificatesDeduplicatesAlgorithms(t *testing.T) {
	certs, err := SelfSignedCertificates(DefaultServerName, CertificateKeyRSA, CertificateKeyRSA, CertificateKeyECDSA)
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) != 2 {
		t.Fatalf("certs=%d, want 2", len(certs))
	}
	first, err := x509.ParseCertificate(certs[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := x509.ParseCertificate(certs[1].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if first.PublicKeyAlgorithm != x509.RSA || second.PublicKeyAlgorithm != x509.ECDSA {
		t.Fatalf("algorithms=%v/%v, want RSA/ECDSA", first.PublicKeyAlgorithm, second.PublicKeyAlgorithm)
	}
}

package main

import (
	cryptotls "crypto/tls"
	"crypto/x509"
	"testing"
)

func TestCertificateAlgorithmsMatchCipherSuiteAuth(t *testing.T) {
	tests := []struct {
		name   string
		suites []uint16
		want   []x509.PublicKeyAlgorithm
	}{
		{
			name:   "default uses ecdsa",
			suites: nil,
			want:   []x509.PublicKeyAlgorithm{x509.ECDSA},
		},
		{
			name:   "rsa suite uses rsa",
			suites: []uint16{cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
			want:   []x509.PublicKeyAlgorithm{x509.RSA},
		},
		{
			name:   "ecdsa suite uses ecdsa",
			suites: []uint16{cryptotls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
			want:   []x509.PublicKeyAlgorithm{x509.ECDSA},
		},
		{
			name: "mixed suites use both",
			suites: []uint16{
				cryptotls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
			want: []x509.PublicKeyAlgorithm{x509.ECDSA, x509.RSA},
		},
	}
	for _, tc := range tests {
		cfg := config{CipherSuiteIDs: tc.suites}
		certs, err := benchmarkCertificates(cfg)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(certs) != len(tc.want) {
			t.Fatalf("%s: certs=%d, want %d", tc.name, len(certs), len(tc.want))
		}
		for i, cert := range certs {
			leaf, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				t.Fatalf("%s: parse certificate %d: %v", tc.name, i, err)
			}
			if leaf.PublicKeyAlgorithm != tc.want[i] {
				t.Fatalf("%s: certificate %d algorithm=%v, want %v", tc.name, i, leaf.PublicKeyAlgorithm, tc.want[i])
			}
		}
	}
}

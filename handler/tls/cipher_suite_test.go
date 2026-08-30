package tls

import (
	"errors"
	"testing"

	cryptotls "crypto/tls"
)

func TestCipherSuiteCatalogIncludesRuntimeSuites(t *testing.T) {
	catalog := CipherSuiteCatalog(CipherSuiteOptions{})
	ecdhe, ok := findCipherSuiteInfo(catalog, cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256)
	if !ok {
		t.Fatal("TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 missing")
	}
	if ecdhe.Name != "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256" || ecdhe.OpenSSLName != "ECDHE-RSA-AES128-GCM-SHA256" {
		t.Fatalf("ecdhe=%+v", ecdhe)
	}
	if !ecdhe.Configurable || ecdhe.Insecure {
		t.Fatalf("ecdhe flags=%+v", ecdhe)
	}

	tls13, ok := findCipherSuiteInfo(catalog, cryptotls.TLS_AES_128_GCM_SHA256)
	if !ok {
		t.Fatal("TLS_AES_128_GCM_SHA256 missing")
	}
	if tls13.Configurable {
		t.Fatalf("tls13=%+v, want non-configurable", tls13)
	}
}

func TestCipherSuiteCatalogRequiresInsecureOptIn(t *testing.T) {
	if _, ok := findCipherSuiteInfo(CipherSuiteCatalog(CipherSuiteOptions{}), cryptotls.TLS_RSA_WITH_AES_128_CBC_SHA); ok {
		t.Fatal("insecure RSA cipher suite leaked into default catalog")
	}
	if _, ok := findCipherSuiteInfo(CipherSuiteCatalog(CipherSuiteOptions{IncludeInsecure: true}), cryptotls.TLS_RSA_WITH_AES_128_CBC_SHA); !ok {
		t.Fatal("insecure RSA cipher suite missing when explicitly enabled")
	}
}

func TestCipherSuiteCatalogReportsCertificateAuth(t *testing.T) {
	tests := []struct {
		name string
		id   uint16
		want CipherSuiteCertificateAuth
	}{
		{
			name: "rsa authenticated",
			id:   cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			want: CipherSuiteCertificateRSA,
		},
		{
			name: "ecdsa authenticated",
			id:   cryptotls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			want: CipherSuiteCertificateECDSA,
		},
		{
			name: "tls13 independent",
			id:   cryptotls.TLS_AES_128_GCM_SHA256,
			want: CipherSuiteCertificateAny,
		},
	}
	for _, tc := range tests {
		info, ok := findCipherSuiteInfo(CipherSuiteCatalog(CipherSuiteOptions{}), tc.id)
		if !ok {
			t.Fatalf("%s missing", tc.name)
		}
		if info.CertificateAuth != tc.want {
			t.Fatalf("%s certificateAuth=%s, want %s", tc.name, info.CertificateAuth, tc.want)
		}
	}
}

func TestLookupCipherSuiteIDHonorsInsecureOptIn(t *testing.T) {
	info, err := LookupCipherSuiteID(cryptotls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, CipherSuiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256" || info.CertificateAuth != CipherSuiteCertificateECDSA {
		t.Fatalf("info=%+v", info)
	}

	_, err = LookupCipherSuiteID(cryptotls.TLS_RSA_WITH_AES_128_CBC_SHA, CipherSuiteOptions{})
	if !errors.Is(err, ErrInsecureCipherSuite) {
		t.Fatalf("err=%v, want %v", err, ErrInsecureCipherSuite)
	}
	info, err = LookupCipherSuiteID(cryptotls.TLS_RSA_WITH_AES_128_CBC_SHA, CipherSuiteOptions{IncludeInsecure: true})
	if err != nil {
		t.Fatal(err)
	}
	if !info.Insecure || info.CertificateAuth != CipherSuiteCertificateRSA {
		t.Fatalf("info=%+v", info)
	}
}

func TestParseCipherSuitesAcceptsJavaOpenSSLandHexNames(t *testing.T) {
	got, err := ParseCipherSuites(" TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:ECDHE-ECDSA-AES256-GCM-SHA384,0xCCA8 ", CipherSuiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []uint16{
		cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		cryptotls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		cryptotls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
	}
	if len(got) != len(want) {
		t.Fatalf("suites=%x, want %x", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("suites=%x, want %x", got, want)
		}
	}
}

func TestParseCipherSuitesDeduplicatesAliases(t *testing.T) {
	got, err := ParseCipherSuites("TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,ECDHE-RSA-AES128-GCM-SHA256,c02f", CipherSuiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 {
		t.Fatalf("suites=%x, want single c02f", got)
	}
}

func TestParseCipherSuitesRejectsUnknownName(t *testing.T) {
	_, err := ParseCipherSuites("TLS_FAKE_WITH_AES_128_GCM_SHA256", CipherSuiteOptions{})
	if !errors.Is(err, ErrUnknownCipherSuite) {
		t.Fatalf("err=%v, want %v", err, ErrUnknownCipherSuite)
	}
}

func TestParseCipherSuitesRejectsInsecureByDefault(t *testing.T) {
	_, err := ParseCipherSuites("TLS_RSA_WITH_AES_128_CBC_SHA", CipherSuiteOptions{})
	if !errors.Is(err, ErrInsecureCipherSuite) {
		t.Fatalf("err=%v, want %v", err, ErrInsecureCipherSuite)
	}

	got, err := ParseCipherSuites("TLS_RSA_WITH_AES_128_CBC_SHA", CipherSuiteOptions{IncludeInsecure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != cryptotls.TLS_RSA_WITH_AES_128_CBC_SHA {
		t.Fatalf("suites=%x, want TLS_RSA_WITH_AES_128_CBC_SHA", got)
	}
}

func TestConfigureCipherSuitesCopiesTLS12Suites(t *testing.T) {
	source := []uint16{cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}
	cfg := &cryptotls.Config{}
	if err := ConfigureCipherSuites(cfg, source); err != nil {
		t.Fatal(err)
	}
	source[0] = cryptotls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
	if len(cfg.CipherSuites) != 1 || cfg.CipherSuites[0] != cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 {
		t.Fatalf("cipherSuites=%x, want copied c02f", cfg.CipherSuites)
	}
}

func TestConfigureCipherSuitesRejectsTLS13Suites(t *testing.T) {
	cfg := &cryptotls.Config{}
	err := ConfigureCipherSuites(cfg, []uint16{cryptotls.TLS_AES_128_GCM_SHA256})
	if !errors.Is(err, ErrTLS13CipherSuiteNotConfigurable) {
		t.Fatalf("err=%v, want %v", err, ErrTLS13CipherSuiteNotConfigurable)
	}
}

func findCipherSuiteInfo(catalog []CipherSuiteInfo, id uint16) (CipherSuiteInfo, bool) {
	for _, suite := range catalog {
		if suite.ID == id {
			return suite, true
		}
	}
	return CipherSuiteInfo{}, false
}

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

func TestTLSConfigUsesSelectedVersion(t *testing.T) {
	cfg, err := parseConfig([]string{"-protocol", "https1", "-tls-version", "1.2"})
	if err != nil {
		t.Fatal(err)
	}
	serverConfig, err := serverTLSConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	clientConfig, err := clientTLSConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if serverConfig.MinVersion != cryptotls.VersionTLS12 || serverConfig.MaxVersion != cryptotls.VersionTLS12 {
		t.Fatalf("server tls version min=%x max=%x", serverConfig.MinVersion, serverConfig.MaxVersion)
	}
	if clientConfig.MinVersion != cryptotls.VersionTLS12 || clientConfig.MaxVersion != cryptotls.VersionTLS12 {
		t.Fatalf("client tls version min=%x max=%x", clientConfig.MinVersion, clientConfig.MaxVersion)
	}
}

func TestTLSConfigUsesCipherSuites(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-protocol", "https1",
		"-tls-version", "1.2",
		"-cipher-suites", "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
	})
	if err != nil {
		t.Fatal(err)
	}
	serverConfig, err := serverTLSConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	clientConfig, err := clientTLSConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(serverConfig.CipherSuites) != 1 || serverConfig.CipherSuites[0] != cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 {
		t.Fatalf("server cipher suites=%x", serverConfig.CipherSuites)
	}
	if len(clientConfig.CipherSuites) != 1 || clientConfig.CipherSuites[0] != cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 {
		t.Fatalf("client cipher suites=%x", clientConfig.CipherSuites)
	}
}

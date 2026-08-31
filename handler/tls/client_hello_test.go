package tls

import (
	cryptotls "crypto/tls"
	"net"
	"testing"
	"time"
)

func TestInspectClientHelloParsesSNIAndALPN(t *testing.T) {
	raw := testClientHelloRecord(t)
	hello, complete, err := InspectClientHello(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !complete {
		t.Fatal("client hello should be complete")
	}
	if hello.ServerName != "api.gnalloy.local" {
		t.Fatalf("serverName=%q", hello.ServerName)
	}
	if len(hello.ALPNProtocols) != 2 || hello.ALPNProtocols[0] != "h2" || hello.ALPNProtocols[1] != "http/1.1" {
		t.Fatalf("alpn=%v", hello.ALPNProtocols)
	}
	if len(hello.CipherSuites) == 0 || len(hello.SupportedVersions) == 0 {
		t.Fatalf("hello=%+v", hello)
	}
}

func TestServerConfigWithClientHelloProviderSelectsConfig(t *testing.T) {
	selected := &cryptotls.Config{NextProtos: []string{"h2"}}
	called := false
	cfg := ServerConfigWithClientHelloProvider(
		&cryptotls.Config{NextProtos: []string{"http/1.1"}},
		ClientHelloProviderFunc(func(hello ClientHello) (*cryptotls.Config, error) {
			called = true
			if hello.ServerName != "api.gnalloy.local" || hello.ALPNProtocols[0] != "h2" {
				t.Fatalf("hello=%+v", hello)
			}
			return selected, nil
		}),
	)

	got, err := cfg.GetConfigForClient(&cryptotls.ClientHelloInfo{
		ServerName:      "api.gnalloy.local",
		SupportedProtos: []string{"h2", "http/1.1"},
		CipherSuites:    []uint16{cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
		SupportedVersions: []uint16{
			cryptotls.VersionTLS13,
			cryptotls.VersionTLS12,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called || got == nil || got.NextProtos[0] != "h2" {
		t.Fatalf("called=%v config=%+v", called, got)
	}
	got.NextProtos[0] = "mutated"
	if selected.NextProtos[0] != "h2" {
		t.Fatal("selected config was not cloned")
	}
}

func testClientHelloRecord(t *testing.T) []byte {
	t.Helper()
	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()

	done := make(chan error, 1)
	go func() {
		conn := cryptotls.Client(clientSide, &cryptotls.Config{
			ServerName:         "api.gnalloy.local",
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2", "http/1.1"},
			MinVersion:         cryptotls.VersionTLS12,
		})
		done <- conn.Handshake()
	}()

	raw := make([]byte, 8192)
	n, err := serverSide.Read(raw)
	if err != nil {
		t.Fatal(err)
	}
	_ = clientSide.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for client handshake goroutine")
	}
	return append([]byte(nil), raw[:n]...)
}

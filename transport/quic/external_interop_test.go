package quic

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExternalInteropHandshake(t *testing.T) {
	addr := os.Getenv("GNALLOY_QUIC_INTEROP_ADDR")
	if addr == "" {
		t.Skip("set GNALLOY_QUIC_INTEROP_ADDR to enable external QUIC interop")
	}
	alpn := os.Getenv("GNALLOY_QUIC_INTEROP_ALPN")
	if alpn == "" {
		alpn = DefaultALPN
	}
	serverName := os.Getenv("GNALLOY_QUIC_INTEROP_SERVER_NAME")
	if serverName == "" {
		serverName = hostnameFromAddress(addr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := DialAddr(ctx, addr, Config{
		TLS: &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: interopInsecureTLS(),
		},
		NextProtos: []string{alpn},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseWithError(0, "interop done")

	state := conn.ConnectionState()
	if state.Version != Version1 {
		t.Fatalf("quic version=%v, want %v", state.Version, Version1)
	}
	if state.TLS.Version != tls.VersionTLS13 {
		t.Fatalf("tls version=%x, want TLS 1.3", state.TLS.Version)
	}
	if state.TLS.NegotiatedProtocol != alpn {
		t.Fatalf("alpn=%q, want %q", state.TLS.NegotiatedProtocol, alpn)
	}

	payload := []byte(os.Getenv("GNALLOY_QUIC_INTEROP_PAYLOAD"))
	if len(payload) == 0 {
		return
	}
	expect := []byte(os.Getenv("GNALLOY_QUIC_INTEROP_EXPECT"))
	if len(expect) == 0 {
		expect = payload
	}
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, expect) {
		t.Fatalf("interop response=%q, want %q", got, expect)
	}
}

func hostnameFromAddress(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || net.ParseIP(host) != nil {
		return ""
	}
	return host
}

func interopInsecureTLS() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("GNALLOY_QUIC_INTEROP_INSECURE")))
	return value == "1" || value == "true" || value == "yes"
}

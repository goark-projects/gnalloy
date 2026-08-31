package quic

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"testing"
	"time"
)

func TestNormalizeConfigClonesTLSAndDefaultsRFC9000(t *testing.T) {
	cert, roots := testCertificate(t, "gnalloy.local")
	srcTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      roots,
	}
	cfg, err := NormalizeConfig(Config{TLS: srcTLS})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TLS == srcTLS {
		t.Fatal("normalized TLS config should be cloned")
	}
	if len(cfg.NextProtos) != 1 || cfg.NextProtos[0] != DefaultALPN {
		t.Fatalf("next protos=%v, want %q", cfg.NextProtos, DefaultALPN)
	}
	if len(cfg.TLS.NextProtos) != 1 || cfg.TLS.NextProtos[0] != DefaultALPN {
		t.Fatalf("tls next protos=%v, want %q", cfg.TLS.NextProtos, DefaultALPN)
	}
	if cfg.TLS.MinVersion != tls.VersionTLS13 {
		t.Fatalf("tls min version=%x, want TLS 1.3", cfg.TLS.MinVersion)
	}
	if len(cfg.Versions) != 1 || cfg.Versions[0] != Version1 {
		t.Fatalf("versions=%v, want QUIC v1", cfg.Versions)
	}
}

func TestNormalizeConfigRejectsInvalidBoundaries(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want error
	}{
		{
			name: "missing tls",
			cfg:  Config{},
			want: ErrMissingTLSConfig,
		},
		{
			name: "tls max below TLS 1.3",
			cfg:  Config{TLS: &tls.Config{MaxVersion: tls.VersionTLS12}},
			want: ErrInvalidTLSConfig,
		},
		{
			name: "empty alpn",
			cfg:  Config{TLS: &tls.Config{}, NextProtos: []string{""}},
			want: ErrInvalidTLSConfig,
		},
		{
			name: "unsupported version",
			cfg:  Config{TLS: &tls.Config{}, Versions: []Version{0xfaceb00c}},
			want: ErrInvalidVersion,
		},
		{
			name: "small initial packet",
			cfg:  Config{TLS: &tls.Config{}, InitialPacketSize: MinInitialPacketSize - 1},
			want: ErrInvalidConfig,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeConfig(tc.cfg)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v, want %v", err, tc.want)
			}
		})
	}
}

func TestListenDialAddrEchoOverRFC9000QUIC(t *testing.T) {
	const alpn = "gnalloy-test"
	cert, roots := testCertificate(t, "gnalloy.local")
	listener, err := ListenAddr("127.0.0.1:0", Config{
		TLS:        &tls.Config{Certificates: []tls.Certificate{cert}},
		NextProtos: []string{alpn},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- serveEcho(ctx, listener)
	}()

	conn, err := DialAddr(ctx, listener.Addr().String(), Config{
		TLS: &tls.Config{
			RootCAs:    roots,
			ServerName: "gnalloy.local",
		},
		NextProtos: []string{alpn},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseWithError(0, "test done")

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

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	payload := []byte("ping over rfc9000")
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
	if !bytes.Equal(got, payload) {
		t.Fatalf("echo=%q, want %q", got, payload)
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not finish echo")
	}
}

func TestListenDialAddrSupportsUnidirectionalStream(t *testing.T) {
	const alpn = "gnalloy-test"
	cert, roots := testCertificate(t, "gnalloy.local")
	listener, err := ListenAddr("127.0.0.1:0", Config{
		TLS:        &tls.Config{Certificates: []tls.Certificate{cert}},
		NextProtos: []string{alpn},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	received := make(chan []byte, 1)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- receiveUniStream(ctx, listener, received)
	}()

	conn, err := DialAddr(ctx, listener.Addr().String(), Config{
		TLS: &tls.Config{
			RootCAs:    roots,
			ServerName: "gnalloy.local",
		},
		NextProtos: []string{alpn},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseWithError(0, "test done")

	stream, err := conn.OpenUniStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	payload := []byte("control stream")
	if _, err := stream.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-received:
		if !bytes.Equal(got, payload) {
			t.Fatalf("uni stream payload=%q, want %q", got, payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for unidirectional stream payload")
	}
	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not finish unidirectional stream")
	}
}

func serveEcho(ctx context.Context, listener Listener) error {
	conn, err := listener.Accept(ctx)
	if err != nil {
		return err
	}
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		return err
	}
	if err := stream.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	if _, err := io.Copy(stream, stream); err != nil {
		return err
	}
	return stream.Close()
}

func receiveUniStream(ctx context.Context, listener Listener, received chan<- []byte) error {
	conn, err := listener.Accept(ctx)
	if err != nil {
		return err
	}
	stream, err := conn.AcceptUniStream(ctx)
	if err != nil {
		return err
	}
	if err := stream.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	payload, err := io.ReadAll(stream)
	if err != nil {
		return err
	}
	received <- payload
	return nil
}

func testCertificate(t *testing.T, name string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{name},
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("failed to add test certificate root")
	}
	return cert, roots
}

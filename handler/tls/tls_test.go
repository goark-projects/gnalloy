package tls

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	cryptotls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestHandlerNegotiatesAndPassesPlaintext(t *testing.T) {
	cert := testCertificate(t)
	clientSink := &pipeSink{}
	serverSink := &pipeSink{}
	client := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), clientSink)
	server := channel.NewLocalChannel(2, buffer.NewHeapAllocator(), serverSink)
	clientSink.peer = server.Pipeline()
	serverSink.peer = client.Pipeline()

	clientRecv := &plainRecorder{}
	serverEcho := &plainEcho{}
	clientTLS := Client(Config{
		TLS: &cryptotls.Config{
			ServerName:         "gnalloy.local",
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2", "http/1.1"},
		},
	})
	if err := client.Pipeline().AddLast("tls", clientTLS); err != nil {
		t.Fatal(err)
	}
	if err := client.Pipeline().AddLast("recorder", clientRecv); err != nil {
		t.Fatal(err)
	}
	serverTLS := Server(Config{
		TLS: &cryptotls.Config{
			Certificates: []cryptotls.Certificate{cert},
			NextProtos:   []string{"h2", "http/1.1"},
		},
	})
	if err := server.Pipeline().AddLast("tls", serverTLS); err != nil {
		t.Fatal(err)
	}
	if err := server.Pipeline().AddLast("echo", serverEcho); err != nil {
		t.Fatal(err)
	}

	server.Pipeline().FireChannelActive()
	client.Pipeline().FireChannelActive()
	writePlain(t, client, "ping")

	clientRecv.waitString(t, "ping")
	if clientRecv.protocol != "h2" {
		t.Fatalf("alpn=%q, want h2 clientHandshake=%v serverHandshake=%v", clientRecv.protocol, clientTLS.handshake, serverTLS.handshake)
	}
}

func writePlain(t *testing.T, ch channel.Channel, src string) {
	t.Helper()
	out, err := ch.Allocator().Acquire(len(src))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := out.WriteBytes([]byte(src)); err != nil {
		out.Release()
		t.Fatal(err)
	}
	if err := ch.WriteAndFlush(out); err != nil {
		t.Fatal(err)
	}
}

type pipeSink struct {
	peer *channel.Pipeline
}

func (s *pipeSink) Write(msg any) error {
	if s.peer == nil {
		return nil
	}
	s.peer.FireChannelRead(msg)
	return nil
}

func (s *pipeSink) Flush() error {
	if s.peer != nil {
		s.peer.FireChannelReadComplete()
	}
	return nil
}

func (s *pipeSink) Close() error { return nil }

type plainRecorder struct {
	buf      bytes.Buffer
	protocol string
}

func (r *plainRecorder) ChannelRead(_ *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		return
	}
	_, _ = r.buf.Write(buf.Bytes())
	buf.Release()
}

func (r *plainRecorder) UserEventTriggered(ctx *channel.HandlerContext, event any) {
	if ev, ok := event.(HandshakeEvent); ok {
		r.protocol = ev.NegotiatedProtocol
	}
	ctx.FireUserEventTriggered(event)
}

func (r *plainRecorder) String() string {
	return r.buf.String()
}

func (r *plainRecorder) waitString(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if r.String() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("plaintext=%q, want %q", r.String(), want)
}

type plainEcho struct{}

func (plainEcho) ChannelRead(ctx *channel.HandlerContext, msg any) {
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := ctx.Channel().WriteAndFlush(buf); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func testCertificate(t *testing.T) cryptotls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "gnalloy.local"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"gnalloy.local"},
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := cryptotls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

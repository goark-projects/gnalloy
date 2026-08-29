package tls

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	cryptotls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
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

func TestStartTLSPassesPlaintextUntilStartEvent(t *testing.T) {
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
		StartTLS: true,
		TLS: &cryptotls.Config{
			ServerName:         "gnalloy.local",
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2"},
		},
	})
	serverTLS := Server(Config{
		StartTLS: true,
		TLS: &cryptotls.Config{
			Certificates: []cryptotls.Certificate{cert},
			NextProtos:   []string{"h2"},
		},
	})
	if err := client.Pipeline().AddLast("tls", clientTLS); err != nil {
		t.Fatal(err)
	}
	if err := client.Pipeline().AddLast("recorder", clientRecv); err != nil {
		t.Fatal(err)
	}
	if err := server.Pipeline().AddLast("tls", serverTLS); err != nil {
		t.Fatal(err)
	}
	if err := server.Pipeline().AddLast("echo", serverEcho); err != nil {
		t.Fatal(err)
	}

	server.Pipeline().FireChannelActive()
	client.Pipeline().FireChannelActive()
	writePlain(t, client, "clear")
	clientRecv.waitString(t, "clear")

	server.Pipeline().FireUserEventTriggered(StartEvent{})
	client.Pipeline().FireUserEventTriggered(StartEvent{})
	writePlain(t, client, "secure")
	clientRecv.waitString(t, "clearsecure")
	if clientRecv.protocol != "h2" {
		t.Fatalf("alpn=%q, want h2", clientRecv.protocol)
	}
}

func TestServerConfigWithSNISelectsDomainConfig(t *testing.T) {
	defaultCert := testCertificateForName(t, "default.local")
	selectedCert := testCertificateForName(t, "selected.local")
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
			ServerName:         "selected.local",
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2", "http/1.1"},
		},
	})
	serverTLS := Server(Config{
		TLS: ServerConfigWithSNI(
			&cryptotls.Config{Certificates: []cryptotls.Certificate{defaultCert}, NextProtos: []string{"http/1.1"}},
			ServerConfigMap(map[string]*cryptotls.Config{
				"selected.local": &cryptotls.Config{Certificates: []cryptotls.Certificate{selectedCert}, NextProtos: []string{"h2"}},
			}),
		),
	})
	if err := client.Pipeline().AddLast("tls", clientTLS); err != nil {
		t.Fatal(err)
	}
	if err := client.Pipeline().AddLast("recorder", clientRecv); err != nil {
		t.Fatal(err)
	}
	if err := server.Pipeline().AddLast("tls", serverTLS); err != nil {
		t.Fatal(err)
	}
	if err := server.Pipeline().AddLast("echo", serverEcho); err != nil {
		t.Fatal(err)
	}

	server.Pipeline().FireChannelActive()
	client.Pipeline().FireChannelActive()
	writePlain(t, client, "sni")

	clientRecv.waitString(t, "sni")
	if clientRecv.protocol != "h2" {
		t.Fatalf("alpn=%q, want h2 from selected SNI config", clientRecv.protocol)
	}
}

func TestHandlerEmitsStapledOCSPResponse(t *testing.T) {
	staple := []byte{0x30, 0x03, 0x0a, 0x01, 0x00}
	cert, err := CertificateWithOCSPStaple(testCertificate(t), staple)
	if err != nil {
		t.Fatal(err)
	}
	clientSink := &pipeSink{}
	serverSink := &pipeSink{}
	client := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), clientSink)
	server := channel.NewLocalChannel(2, buffer.NewHeapAllocator(), serverSink)
	clientSink.peer = server.Pipeline()
	serverSink.peer = client.Pipeline()

	validatorCalled := false
	clientRecv := &plainRecorder{}
	serverEcho := &plainEcho{}
	clientTLS := Client(Config{
		OCSP: OCSPConfig{
			RequireStaple: true,
			EmitEvent:     true,
			Validator: OCSPValidatorFunc(func(state cryptotls.ConnectionState, response []byte) error {
				validatorCalled = true
				if state.ServerName != "gnalloy.local" {
					t.Fatalf("serverName=%q, want gnalloy.local", state.ServerName)
				}
				if !bytes.Equal(response, staple) {
					t.Fatalf("ocsp=%x, want %x", response, staple)
				}
				return nil
			}),
		},
		TLS: &cryptotls.Config{
			ServerName:         "gnalloy.local",
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2"},
		},
	})
	serverTLS := Server(Config{
		TLS: &cryptotls.Config{
			Certificates: []cryptotls.Certificate{cert},
			NextProtos:   []string{"h2"},
		},
	})
	if err := client.Pipeline().AddLast("tls", clientTLS); err != nil {
		t.Fatal(err)
	}
	if err := client.Pipeline().AddLast("recorder", clientRecv); err != nil {
		t.Fatal(err)
	}
	if err := server.Pipeline().AddLast("tls", serverTLS); err != nil {
		t.Fatal(err)
	}
	if err := server.Pipeline().AddLast("echo", serverEcho); err != nil {
		t.Fatal(err)
	}

	server.Pipeline().FireChannelActive()
	client.Pipeline().FireChannelActive()
	writePlain(t, client, "ocsp")

	clientRecv.waitString(t, "ocsp")
	if !validatorCalled {
		t.Fatal("OCSP validator was not called")
	}
	if len(clientRecv.ocsp) != 1 {
		t.Fatalf("ocsp events=%d, want 1", len(clientRecv.ocsp))
	}
	event := clientRecv.ocsp[0]
	if !event.Stapled || !event.Validated || !bytes.Equal(event.Response, staple) {
		t.Fatalf("ocsp event=%+v", event)
	}
	staple[0] = 0xff
	if event.Response[0] == 0xff {
		t.Fatal("OCSP event response aliases caller memory")
	}
}

func TestHandlerRequiresOCSPStapleRejectsMissingResponse(t *testing.T) {
	cert := testCertificate(t)
	clientSink := &pipeSink{}
	serverSink := &pipeSink{}
	client := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), clientSink)
	server := channel.NewLocalChannel(2, buffer.NewHeapAllocator(), serverSink)
	clientSink.peer = server.Pipeline()
	serverSink.peer = client.Pipeline()

	errorsSeen := &errorRecorder{ch: make(chan error, 4)}
	clientTLS := Client(Config{
		OCSP: OCSPConfig{RequireStaple: true},
		TLS: &cryptotls.Config{
			ServerName:         "gnalloy.local",
			InsecureSkipVerify: true,
		},
	})
	serverTLS := Server(Config{TLS: &cryptotls.Config{Certificates: []cryptotls.Certificate{cert}}})
	if err := client.Pipeline().AddLast("tls", clientTLS); err != nil {
		t.Fatal(err)
	}
	if err := client.Pipeline().AddLast("errors", errorsSeen); err != nil {
		t.Fatal(err)
	}
	if err := server.Pipeline().AddLast("tls", serverTLS); err != nil {
		t.Fatal(err)
	}
	if err := server.Pipeline().AddLast("echo", plainEcho{}); err != nil {
		t.Fatal(err)
	}

	server.Pipeline().FireChannelActive()
	client.Pipeline().FireChannelActive()
	writePlain(t, client, "ping")

	select {
	case err := <-errorsSeen.ch:
		if !errors.Is(err, ErrOCSPStapleRequired) {
			t.Fatalf("err=%v, want ErrOCSPStapleRequired", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for OCSP required error")
	}
}

func TestHandlerVerifyPeerNameRejectsMismatchedCertificate(t *testing.T) {
	cert := testCertificate(t)
	clientSink := &pipeSink{}
	serverSink := &pipeSink{}
	client := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), clientSink)
	server := channel.NewLocalChannel(2, buffer.NewHeapAllocator(), serverSink)
	clientSink.peer = server.Pipeline()
	serverSink.peer = client.Pipeline()

	errorsSeen := &errorRecorder{ch: make(chan error, 4)}
	clientTLS := Client(Config{
		VerifyPeerName: "other.local",
		TLS: &cryptotls.Config{
			ServerName:         "gnalloy.local",
			InsecureSkipVerify: true,
		},
	})
	serverTLS := Server(Config{TLS: &cryptotls.Config{Certificates: []cryptotls.Certificate{cert}}})
	if err := client.Pipeline().AddLast("tls", clientTLS); err != nil {
		t.Fatal(err)
	}
	if err := client.Pipeline().AddLast("errors", errorsSeen); err != nil {
		t.Fatal(err)
	}
	if err := server.Pipeline().AddLast("tls", serverTLS); err != nil {
		t.Fatal(err)
	}
	if err := server.Pipeline().AddLast("echo", plainEcho{}); err != nil {
		t.Fatal(err)
	}

	server.Pipeline().FireChannelActive()
	client.Pipeline().FireChannelActive()
	writePlain(t, client, "ping")

	select {
	case err := <-errorsSeen.ch:
		if err == nil || errors.Is(err, ErrPeerCertificateUnavailable) {
			t.Fatalf("err=%v, want hostname verification error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for hostname verification error")
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
	ocsp     []OCSPEvent
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
	switch ev := event.(type) {
	case HandshakeEvent:
		r.protocol = ev.NegotiatedProtocol
	case OCSPEvent:
		r.ocsp = append(r.ocsp, ev)
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
	return testCertificateForName(t, "gnalloy.local")
}

func testCertificateForName(t *testing.T, name string) cryptotls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
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
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := cryptotls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

type errorRecorder struct {
	ch chan error
}

func (r *errorRecorder) ExceptionCaught(_ *channel.HandlerContext, err error) {
	r.ch <- err
}

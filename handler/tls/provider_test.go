package tls

import (
	cryptotls "crypto/tls"
	"errors"
	"net"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestHandlerUsesConfiguredProvider(t *testing.T) {
	cert := testCertificate(t)
	clientProvider := newRecordingProvider("client-provider")
	serverProvider := newRecordingProvider("server-provider")
	clientSink := &pipeSink{}
	serverSink := &pipeSink{}
	client := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), clientSink)
	server := channel.NewLocalChannel(2, buffer.NewHeapAllocator(), serverSink)
	clientSink.peer = server.Pipeline()
	serverSink.peer = client.Pipeline()

	clientRecv := &plainRecorder{}
	serverEcho := &plainEcho{}
	clientTLS := Client(Config{
		Provider: clientProvider,
		TLS: &cryptotls.Config{
			ServerName:         "gnalloy.local",
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2", "http/1.1"},
		},
	})
	serverTLS := Server(Config{
		Provider: serverProvider,
		TLS: &cryptotls.Config{
			Certificates: []cryptotls.Certificate{cert},
			NextProtos:   []string{"h2", "http/1.1"},
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
	writePlain(t, client, "ping")

	clientRecv.waitString(t, "ping")
	if clientProvider.clientCalls != 1 || serverProvider.serverCalls != 1 {
		t.Fatalf("provider calls client=%d server=%d", clientProvider.clientCalls, serverProvider.serverCalls)
	}
	if clientRecv.protocol != "h2" {
		t.Fatalf("alpn=%q, want h2", clientRecv.protocol)
	}
}

func TestHandlerRejectsUnsupportedProvider(t *testing.T) {
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	provider := newRecordingProvider("bad")
	provider.capabilities.TLS13 = false
	err := ch.Pipeline().AddLast("tls", Client(Config{Provider: provider}))
	if !errors.Is(err, ErrNativeTLSUnavailable) {
		t.Fatalf("err=%v, want ErrNativeTLSUnavailable", err)
	}
}

func TestEvaluateProviderDoesNotRequireQUICPacketProtection(t *testing.T) {
	evaluation := EvaluateProvider(newRecordingProvider("handler-provider"))
	if !evaluation.Supported {
		t.Fatalf("evaluation=%+v, want supported", evaluation)
	}
}

func TestEvaluateProviderRejectsTypedNil(t *testing.T) {
	var provider *recordingProvider
	evaluation := EvaluateProvider(provider)
	if evaluation.Supported {
		t.Fatalf("evaluation=%+v, want unsupported", evaluation)
	}
}

type recordingProvider struct {
	capabilities NativeCapabilities
	clientCalls  int
	serverCalls  int
}

func newRecordingProvider(name string) *recordingProvider {
	return &recordingProvider{
		capabilities: NativeCapabilities{
			Provider: name,
			TLS13:    true,
			ALPN:     true,
			SNI:      true,
		},
	}
}

func (p *recordingProvider) Capabilities() NativeCapabilities {
	if p == nil {
		return NativeCapabilities{}
	}
	return p.capabilities
}

func (p *recordingProvider) Client(conn net.Conn, cfg *cryptotls.Config) (Conn, error) {
	p.clientCalls++
	return cryptotls.Client(conn, cfg), nil
}

func (p *recordingProvider) Server(conn net.Conn, cfg *cryptotls.Config) (Conn, error) {
	p.serverCalls++
	return cryptotls.Server(conn, cfg), nil
}

package http3

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
	"strconv"
	"testing"
	"time"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	codechttp3 "goark.dev/gnalloy/codec/http3"
	"goark.dev/gnalloy/transport/quic/rfc9000"
)

func TestSessionRoundTripOverRFC9000QUIC(t *testing.T) {
	const alpn = "h3"
	cert, roots := http3TestCertificate(t, "gnalloy.local")
	listener, err := rfc9000.ListenAddr("127.0.0.1:0", rfc9000.Config{
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
		serverErr <- serveHTTP3Once(ctx, listener, []byte("h3-response"))
	}()

	conn, err := rfc9000.DialAddr(ctx, listener.Addr().String(), rfc9000.Config{
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
	if got := conn.ConnectionState().TLS.NegotiatedProtocol; got != alpn {
		t.Fatalf("alpn=%q, want %q", got, alpn)
	}

	session, err := NewSession(conn, Config{})
	if err != nil {
		t.Fatal(err)
	}
	streamCh, err := session.OpenRequestStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer streamCh.Close()

	capture := &http3ResponseCapture{}
	if err := streamCh.Channel().Pipeline().AddLast("capture", capture); err != nil {
		t.Fatal(err)
	}
	if err := streamCh.Channel().WriteAndFlush(codechttp3.HeadersBlock{Fields: []codechttp3.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "gnalloy.local"},
		{Name: ":path", Value: "/benchmark"},
	}}); err != nil {
		t.Fatal(err)
	}

	if err := readUntilHTTP3Response(ctx, streamCh, capture, []byte("h3-response")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func serveHTTP3Once(ctx context.Context, listener rfc9000.Listener, body []byte) error {
	conn, err := listener.Accept(ctx)
	if err != nil {
		return err
	}
	defer conn.CloseWithError(0, "server done")
	session, err := NewSession(conn, Config{})
	if err != nil {
		return err
	}
	streamCh, err := session.AcceptRequestStream(ctx)
	if err != nil {
		return err
	}
	if err := streamCh.Channel().Pipeline().AddLast("handler", http3ResponseHandler{body: body}); err != nil {
		return err
	}
	_, err = streamCh.ReadOnce(ctx)
	if err != nil && !errors.Is(err, io.EOF) {
		_ = streamCh.Close()
		return err
	}
	return streamCh.Close()
}

type http3ResponseHandler struct {
	body []byte
}

func (h http3ResponseHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if _, ok := msg.(codechttp3.HeadersBlock); !ok {
		ctx.FireChannelRead(msg)
		return
	}
	body, err := ctx.Channel().Allocator().Acquire(len(h.body))
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if _, err := body.WriteBytes(h.body); err != nil {
		body.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Write(codechttp3.HeadersBlock{Fields: []codechttp3.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-length", Value: strconv.Itoa(len(h.body))},
	}}); err != nil {
		body.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Write(codechttp3.DataFrame{Data: body}); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Flush(); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

type http3ResponseCapture struct {
	status string
	body   bytes.Buffer
}

func (c *http3ResponseCapture) ChannelRead(_ *channel.HandlerContext, msg any) {
	switch frame := msg.(type) {
	case codechttp3.HeadersBlock:
		for _, field := range frame.Fields {
			if field.Name == ":status" {
				c.status = field.Value
			}
		}
	case codechttp3.DataFrame:
		copyFrameData(&c.body, frame.Data)
		frame.Release()
	default:
		releaseHTTP3Message(msg)
	}
}

func readUntilHTTP3Response(ctx context.Context, streamCh *StreamChannel, capture *http3ResponseCapture, want []byte) error {
	for {
		if capture.status == "200" && bytes.Equal(capture.body.Bytes(), want) {
			return nil
		}
		_, err := streamCh.ReadOnce(ctx)
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) && capture.status == "200" && bytes.Equal(capture.body.Bytes(), want) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func copyFrameData(dst *bytes.Buffer, src buffer.ByteBuf) {
	if src == nil {
		return
	}
	var stack [8][]byte
	for _, segment := range src.ReadableSlices(stack[:0]) {
		_, _ = dst.Write(segment)
	}
}

func releaseHTTP3Message(msg any) {
	if releaser, ok := msg.(interface{ Release() }); ok {
		releaser.Release()
	}
}

func http3TestCertificate(t *testing.T, name string) (tls.Certificate, *x509.CertPool) {
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
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("failed to add root certificate")
	}
	return cert, roots
}

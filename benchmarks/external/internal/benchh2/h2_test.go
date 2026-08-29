package benchh2

import (
	"bytes"
	"context"
	cryptotls "crypto/tls"
	"io"
	"net"
	"testing"
	"time"

	"goark.dev/gnalloy/benchmarks/external/internal/benchtls"
)

func TestRequestHeaderBlockUsesStaticHPACKFields(t *testing.T) {
	block := requestHeaderBlock("localhost", false)
	if !bytes.HasPrefix(block, []byte{0x82, 0x86, 0x04, 0x06}) {
		t.Fatalf("header block prefix=%x", block[:4])
	}
	if !bytes.Contains(block, []byte("/bench")) || !bytes.Contains(block, []byte("localhost")) {
		t.Fatalf("header block=%x", block)
	}
}

func TestRunLoadHTTP2Cleartext(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := serveRawH2(t, ln, 16, 3)

	result, err := RunLoad(context.Background(), Config{
		Addr:              ln.Addr().String(),
		Payload:           16,
		Connections:       1,
		Messages:          2,
		WarmupMessages:    1,
		LatencySampleRate: 1,
		Timeout:           5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 2 || result.Errors != 0 || result.Latency.Samples != 2 {
		t.Fatalf("result=%+v", result)
	}
	<-done
}

func TestRunLoadHTTP2TLSALPN(t *testing.T) {
	cert, err := benchtls.SelfSignedCertificate(benchtls.DefaultServerName)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := cryptotls.Listen("tcp", "127.0.0.1:0", &cryptotls.Config{
		Certificates: []cryptotls.Certificate{cert},
		MinVersion:   cryptotls.VersionTLS13,
		NextProtos:   []string{"h2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := serveRawH2(t, ln, 8, 3)

	result, err := RunLoad(context.Background(), Config{
		Addr:              ln.Addr().String(),
		ServerName:        benchtls.DefaultServerName,
		Payload:           8,
		Connections:       1,
		Messages:          2,
		WarmupMessages:    1,
		LatencySampleRate: 1,
		Timeout:           5 * time.Second,
		TLS: &cryptotls.Config{
			ServerName:         benchtls.DefaultServerName,
			InsecureSkipVerify: true,
			MinVersion:         cryptotls.VersionTLS13,
			NextProtos:         []string{"h2"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 2 || result.Errors != 0 || result.NegotiatedProtocol != "h2" {
		t.Fatalf("result=%+v", result)
	}
	<-done
}

func TestRunLoadTimeoutClosesBlockedClients(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		<-stop
	}()
	defer func() {
		close(stop)
		_ = ln.Close()
		<-done
	}()

	start := time.Now()
	_, err = RunLoad(context.Background(), Config{
		Addr:        ln.Addr().String(),
		Payload:     8,
		Connections: 1,
		Messages:    1,
		Timeout:     150 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout or closed connection error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("RunLoad elapsed=%v, want bounded timeout", elapsed)
	}
}

func serveRawH2(t *testing.T, ln net.Listener, payload int, requests int) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if err := serveRawH2Conn(conn, payload, requests); err != nil {
			t.Errorf("serve h2: %v", err)
		}
	}()
	return done
}

func serveRawH2Conn(conn net.Conn, payload int, requests int) error {
	preface := make([]byte, len(clientPreface))
	if _, err := io.ReadFull(conn, preface); err != nil {
		return err
	}
	if !bytes.Equal(preface, clientPreface) {
		return io.ErrUnexpectedEOF
	}
	if err := writeFrame(conn, frameSettings, 0, 0, nil); err != nil {
		return err
	}
	body := ResponseBody(payload)
	served := 0
	for served < requests {
		header, err := readFrameHeader(conn)
		if err != nil {
			return err
		}
		switch header.typ {
		case frameSettings:
			if err := skipFramePayload(conn, header.length); err != nil {
				return err
			}
			if header.flags&flagAck == 0 {
				if err := writeSettingsAck(conn); err != nil {
					return err
				}
			}
		case frameWindowUpdate:
			if err := skipFramePayload(conn, header.length); err != nil {
				return err
			}
		case frameHeaders:
			if err := skipFramePayload(conn, header.length); err != nil {
				return err
			}
			if err := writeFrame(conn, frameHeaders, flagEndHeaders, header.streamID, []byte{0x88}); err != nil {
				return err
			}
			if err := writeFrame(conn, frameData, flagEndStream, header.streamID, body); err != nil {
				return err
			}
			served++
		default:
			if err := skipFramePayload(conn, header.length); err != nil {
				return err
			}
		}
	}
	return nil
}

package benchhttp

import (
	"bufio"
	"context"
	cryptotls "crypto/tls"
	"net"
	"strings"
	"testing"
	"time"

	"goark.dev/gnalloy/benchmarks/external/internal/benchtls"
)

func TestServerStateCountsSplitRequests(t *testing.T) {
	var state ServerState
	req := RequestBytes("localhost")
	if got := state.AppendAndCountRequests(req[:5]); got != 0 {
		t.Fatalf("requests=%d, want 0", got)
	}
	if got := state.AppendAndCountRequests(append(req[5:], req...)); got != 2 {
		t.Fatalf("requests=%d, want 2", got)
	}
}

func TestResponseBytesUsesRequestedPayload(t *testing.T) {
	resp := string(ResponseBytes(8))
	if !strings.Contains(resp, "Content-Length: 8\r\n") {
		t.Fatalf("response=%q", resp)
	}
	if len(resp) <= 8 || resp[len(resp)-8] != 0 {
		t.Fatalf("response body not appended: %q", resp)
	}
}

func TestRunLoadHTTP1(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	response := ResponseBytes(16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for i := 0; i < 3; i++ {
			for {
				line, err := reader.ReadString('\n')
				if err != nil || line == "\r\n" {
					break
				}
			}
			_, _ = conn.Write(response)
		}
	}()

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

func TestRunLoadHTTPS1ALPN(t *testing.T) {
	cert, err := benchtls.SelfSignedCertificate(benchtls.DefaultServerName)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := cryptotls.Listen("tcp", "127.0.0.1:0", &cryptotls.Config{
		Certificates: []cryptotls.Certificate{cert},
		MinVersion:   cryptotls.VersionTLS13,
		NextProtos:   []string{"http/1.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	response := ResponseBytes(8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for i := 0; i < 3; i++ {
			for {
				line, err := reader.ReadString('\n')
				if err != nil || line == "\r\n" {
					break
				}
			}
			_, _ = conn.Write(response)
		}
	}()

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
			NextProtos:         []string{"http/1.1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 2 || result.Errors != 0 || result.NegotiatedProtocol != "http/1.1" {
		t.Fatalf("result=%+v", result)
	}
	<-done
}

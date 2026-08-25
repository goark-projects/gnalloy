package smokeclient

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestExchangeRaw(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		var buf [4]byte
		_, _ = io.ReadFull(server, buf[:])
		_, _ = server.Write(buf[:])
	}()

	if err := exchange(client, ProtocolRaw, []byte("ping")); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeLengthField(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		payload, err := readFrame(server)
		if err != nil {
			return
		}
		_ = writeFrame(server, payload)
	}()

	if err := exchange(client, ProtocolLengthField, []byte("ping")); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeLine(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		got, err := readLine(server)
		if err != nil {
			return
		}
		_, _ = server.Write(appendLine(got))
	}()

	if err := exchange(client, ProtocolLine, []byte("ping")); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeFixed(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		var buf [4]byte
		_, _ = io.ReadFull(server, buf[:])
		_, _ = server.Write(buf[:])
	}()

	if err := exchange(client, ProtocolFixed, []byte("ping")); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeRejectsInvalidProtocol(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	err := exchange(client, Protocol("bad"), []byte("ping"))
	if !errors.Is(err, ErrInvalidProtocol) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidProtocol)
	}
}

func TestRunUDP(t *testing.T) {
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		var buf [16]byte
		n, addr, err := server.ReadFrom(buf[:])
		if err != nil {
			return
		}
		_, _ = server.WriteTo(buf[:n], addr)
	}()

	if err := Run(nil, Config{
		Addr:     server.LocalAddr().String(),
		Protocol: ProtocolUDP,
		Message:  []byte("ping"),
		Count:    1,
		Timeout:  time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	<-done
}

package smokeclient

import (
	"errors"
	"io"
	"net"
	"strings"
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

func TestExchangeHTTP1(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		req, _ := readHTTPHeader(server)
		if !strings.HasPrefix(req, "GET / HTTP/1.1\r\n") {
			return
		}
		_, _ = server.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 14\r\n\r\ngnalloy http1\n"))
	}()

	if err := exchange(client, ProtocolHTTP1, []byte("ping")); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeWebSocket(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		req, _ := readHTTPHeader(server)
		if !strings.Contains(req, "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n") {
			return
		}
		_, _ = server.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n\r\n"))
		opcode, payload, err := readMaskedWebSocketFrame(server)
		if err != nil || opcode != 0x1 {
			return
		}
		_, _ = server.Write([]byte{0x81, byte(len(payload))})
		_, _ = server.Write(payload)
		_, _, _ = readMaskedWebSocketFrame(server)
	}()

	if err := exchange(client, ProtocolWebSocket, []byte("ping")); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeMQTT(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		_, _, err := readMQTTFrame(server)
		if err != nil {
			return
		}
		_, _ = server.Write([]byte{0x20, 0x02, 0x00, 0x00})
		header, body, err := readMQTTFrame(server)
		if err != nil || header != 0xc0 || len(body) != 0 {
			return
		}
		_, _ = server.Write([]byte{0xd0, 0x00})
		header, body, err = readMQTTFrame(server)
		if err != nil || header != 0x30 {
			return
		}
		_, _ = server.Write(appendMQTTFrame(nil, header, body))
		_, _, _ = readMQTTFrame(server)
	}()

	if err := exchange(client, ProtocolMQTT, []byte("ping")); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeRedis(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		req, _ := readRESPBytes(server, len("*1\r\n$4\r\nPING\r\n*1\r\n$4\r\nPING\r\n"))
		if string(req) != "*1\r\n$4\r\nPING\r\n*1\r\n$4\r\nPING\r\n" {
			return
		}
		_, _ = server.Write([]byte("+PONG\r\n+PONG\r\n"))
	}()

	if err := exchange(client, ProtocolRedis, []byte("ping")); err != nil {
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

func readHTTPHeader(r io.Reader) (string, error) {
	out := make([]byte, 0, 128)
	var one [1]byte
	for {
		if _, err := io.ReadFull(r, one[:]); err != nil {
			return "", err
		}
		out = append(out, one[0])
		if len(out) >= 4 && string(out[len(out)-4:]) == "\r\n\r\n" {
			return string(out), nil
		}
	}
}

func readMaskedWebSocketFrame(r io.Reader) (byte, []byte, error) {
	var header [6]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	length := int(header[1] & 0x7f)
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	for i := range payload {
		payload[i] ^= header[2+(i&3)]
	}
	return header[0] & 0x0f, payload, nil
}

func readRESPBytes(r io.Reader, n int) ([]byte, error) {
	out := make([]byte, n)
	_, err := io.ReadFull(r, out)
	return out, err
}

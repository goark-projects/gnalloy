package smokeclient

import (
	"bytes"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRunWebSocketUsesSingleSession(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	done := make(chan error, 1)
	go func() {
		done <- serveWebSocketSession(ln, []byte("ping"), 3)
	}()

	if err := Run(nil, Config{
		Addr:     ln.Addr().String(),
		Protocol: ProtocolWebSocket,
		Message:  []byte("ping"),
		Count:    3,
		Timeout:  time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRunMQTTUsesSingleSession(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	done := make(chan error, 1)
	go func() {
		done <- serveMQTTSession(ln, []byte("ping"), 3)
	}()

	if err := Run(nil, Config{
		Addr:     ln.Addr().String(),
		Protocol: ProtocolMQTT,
		Message:  []byte("ping"),
		Count:    3,
		Timeout:  time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func serveWebSocketSession(ln net.Listener, want []byte, count int) error {
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()

	req, err := readHTTPHeader(conn)
	if err != nil {
		return err
	}
	if !strings.Contains(req, "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n") {
		return errors.New("missing websocket key")
	}
	if _, err := conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n\r\n")); err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		opcode, payload, err := readMaskedWebSocketFrame(conn)
		if err != nil {
			return err
		}
		if opcode != 0x1 || !bytes.Equal(payload, want) {
			return errors.New("unexpected websocket frame")
		}
		if _, err := conn.Write([]byte{0x81, byte(len(payload))}); err != nil {
			return err
		}
		if _, err := conn.Write(payload); err != nil {
			return err
		}
	}
	opcode, payload, err := readMaskedWebSocketFrame(conn)
	if err != nil {
		return err
	}
	if opcode != 0x8 || !bytes.Equal(payload, []byte{0x03, 0xe8}) {
		return errors.New("unexpected websocket close")
	}
	return nil
}

func serveMQTTSession(ln net.Listener, want []byte, count int) error {
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()

	header, _, err := readMQTTFrame(conn)
	if err != nil {
		return err
	}
	if header != 0x10 {
		return errors.New("unexpected mqtt connect")
	}
	if _, err := conn.Write([]byte{0x20, 0x02, 0x00, 0x00}); err != nil {
		return err
	}
	header, body, err := readMQTTFrame(conn)
	if err != nil {
		return err
	}
	if header != 0xc0 || len(body) != 0 {
		return errors.New("unexpected mqtt pingreq")
	}
	if _, err := conn.Write([]byte{0xd0, 0x00}); err != nil {
		return err
	}
	wantPublish := make([]byte, 0, 3+len(want))
	wantPublish = append(wantPublish, 0x00, 0x01, 'x')
	wantPublish = append(wantPublish, want...)
	for i := 0; i < count; i++ {
		header, body, err = readMQTTFrame(conn)
		if err != nil {
			return err
		}
		if header != 0x30 || !bytes.Equal(body, wantPublish) {
			return errors.New("unexpected mqtt publish")
		}
		if _, err := conn.Write(appendMQTTFrame(nil, header, body)); err != nil {
			return err
		}
	}
	header, body, err = readMQTTFrame(conn)
	if err != nil {
		return err
	}
	if header != 0xe0 || len(body) != 0 {
		return errors.New("unexpected mqtt disconnect")
	}
	return nil
}

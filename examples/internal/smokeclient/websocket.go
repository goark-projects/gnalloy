package smokeclient

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strings"
)

func exchangeWebSocket(conn net.Conn, payload []byte) error {
	return runWebSocketClientSession(conn, payload, 1)
}

func runWebSocketClientSession(conn net.Conn, payload []byte, count int) error {
	if err := openWebSocketClientSession(conn); err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		if err := exchangeWebSocketFrame(conn, payload); err != nil {
			return fmt.Errorf("exchange %d: %w", i+1, err)
		}
	}
	return writeWebSocketFrame(conn, 0x8, []byte{0x03, 0xe8})
}

func openWebSocketClientSession(conn net.Conn) error {
	const key = "dGhlIHNhbXBsZSBub25jZQ=="
	req := "GET / HTTP/1.1\r\n" +
		"Host: gnalloy.local\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if err := writeAll(conn, []byte(req)); err != nil {
		return err
	}
	status, headers, _, err := readHTTPResponse(conn)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(status, "HTTP/1.1 101 ") {
		return fmt.Errorf("websocket status=%q, want 101", status)
	}
	if got, want := headers["sec-websocket-accept"], websocketAcceptKey(key); got != want {
		return fmt.Errorf("websocket accept=%q, want %q", got, want)
	}
	return nil
}

func exchangeWebSocketFrame(conn net.Conn, payload []byte) error {
	if err := writeWebSocketFrame(conn, 0x1, payload); err != nil {
		return err
	}
	opcode, got, err := readWebSocketFrame(conn)
	if err != nil {
		return err
	}
	if opcode != 0x1 || !bytes.Equal(got, payload) {
		return fmt.Errorf("websocket echo opcode=%x payload=%q", opcode, got)
	}
	return nil
}

func writeWebSocketFrame(w io.Writer, opcode byte, payload []byte) error {
	if len(payload) > 125 {
		return fmt.Errorf("websocket smoke payload too large: %d", len(payload))
	}
	mask := [4]byte{1, 2, 3, 4}
	frame := make([]byte, 0, 6+len(payload))
	frame = append(frame, 0x80|opcode, 0x80|byte(len(payload)))
	frame = append(frame, mask[:]...)
	for i, b := range payload {
		frame = append(frame, b^mask[i&3])
	}
	return writeAll(w, frame)
}

func readWebSocketFrame(r io.Reader) (byte, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	final := header[0]&0x80 != 0
	opcode := header[0] & 0x0f
	masked := header[1]&0x80 != 0
	length := int(header[1] & 0x7f)
	if !final || masked || length > 125 {
		return 0, nil, fmt.Errorf("invalid websocket smoke frame final=%v masked=%v length=%d", final, masked, length)
	}
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}
	return opcode, payload, nil
}

func websocketAcceptKey(key string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(key) + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}

package smokeclient

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type Protocol string

const (
	ProtocolRaw         Protocol = "raw"
	ProtocolLengthField Protocol = "length-field"
	ProtocolUDP         Protocol = "udp"
	ProtocolLine        Protocol = "line"
	ProtocolFixed       Protocol = "fixed"
	ProtocolHTTP1       Protocol = "http1"
	ProtocolWebSocket   Protocol = "websocket"
	ProtocolMQTT        Protocol = "mqtt"
	ProtocolRedis       Protocol = "redis"
)

var (
	ErrInvalidProtocol = errors.New("gnalloy/examples: invalid smoke protocol")
	ErrInvalidCount    = errors.New("gnalloy/examples: invalid smoke count")
)

type Config struct {
	Addr     string
	Protocol Protocol
	Message  []byte
	Count    int
	Timeout  time.Duration
}

// Run 建立本地连接并验证服务端 echo 行为，供本机与远端平台冒烟使用。
func Run(ctx context.Context, cfg Config) error {
	if cfg.Addr == "" {
		return fmt.Errorf("empty address")
	}
	if cfg.Count <= 0 {
		return ErrInvalidCount
	}
	if len(cfg.Message) == 0 {
		cfg.Message = []byte("ping")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Second
	}
	if err := validateProtocol(cfg.Protocol); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	network := "tcp"
	if cfg.Protocol == ProtocolUDP {
		network = "udp"
	}
	dialer := net.Dialer{Timeout: cfg.Timeout}
	conn, err := dialer.DialContext(ctx, network, cfg.Addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(cfg.Timeout)); err != nil {
		return err
	}

	for i := 0; i < cfg.Count; i++ {
		if err := exchange(conn, cfg.Protocol, cfg.Message); err != nil {
			return fmt.Errorf("exchange %d: %w", i+1, err)
		}
	}
	return nil
}

func exchange(conn net.Conn, protocol Protocol, payload []byte) error {
	switch protocol {
	case ProtocolRaw, ProtocolUDP, ProtocolFixed:
		if err := writeAll(conn, payload); err != nil {
			return err
		}
		got := make([]byte, len(payload))
		if _, err := io.ReadFull(conn, got); err != nil {
			return err
		}
		if string(got) != string(payload) {
			return fmt.Errorf("raw echo=%q, want %q", got, payload)
		}
		return nil
	case ProtocolLine:
		if err := writeAll(conn, appendLine(payload)); err != nil {
			return err
		}
		got, err := readLine(conn)
		if err != nil {
			return err
		}
		if string(got) != string(payload) {
			return fmt.Errorf("line echo=%q, want %q", got, payload)
		}
		return nil
	case ProtocolLengthField:
		if err := writeFrame(conn, payload); err != nil {
			return err
		}
		got, err := readFrame(conn)
		if err != nil {
			return err
		}
		if string(got) != string(payload) {
			return fmt.Errorf("length-field echo=%q, want %q", got, payload)
		}
		return nil
	case ProtocolHTTP1:
		return exchangeHTTP1(conn)
	case ProtocolWebSocket:
		return exchangeWebSocket(conn, payload)
	case ProtocolMQTT:
		return exchangeMQTT(conn, payload)
	case ProtocolRedis:
		return exchangeRedis(conn)
	default:
		return ErrInvalidProtocol
	}
}

func validateProtocol(protocol Protocol) error {
	switch protocol {
	case ProtocolRaw, ProtocolLengthField, ProtocolUDP, ProtocolLine, ProtocolFixed, ProtocolHTTP1, ProtocolWebSocket, ProtocolMQTT, ProtocolRedis:
		return nil
	default:
		return ErrInvalidProtocol
	}
}

func writeFrame(w io.Writer, payload []byte) error {
	if uint64(len(payload)) > uint64(^uint32(0)) {
		return fmt.Errorf("payload too large: %d", len(payload))
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if err := writeAll(w, header[:]); err != nil {
		return err
	}
	return writeAll(w, payload)
}

func readFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header[:])
	out := make([]byte, size)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, err
	}
	return out, nil
}

func exchangeHTTP1(conn net.Conn) error {
	if err := writeAll(conn, []byte("GET / HTTP/1.1\r\nHost: gnalloy.local\r\nConnection: keep-alive\r\n\r\n")); err != nil {
		return err
	}
	status, headers, body, err := readHTTPResponse(conn)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(status, "HTTP/1.1 200 ") {
		return fmt.Errorf("http status=%q, want 200", status)
	}
	if got := string(body); got != "gnalloy http1\n" {
		return fmt.Errorf("http body=%q", got)
	}
	if headers["content-length"] == "" {
		return fmt.Errorf("missing content-length")
	}
	return nil
}

func exchangeWebSocket(conn net.Conn, payload []byte) error {
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
	return writeWebSocketFrame(conn, 0x8, []byte{0x03, 0xe8})
}

func exchangeMQTT(conn net.Conn, payload []byte) error {
	connect := []byte{
		0x10, 0x0f,
		0x00, 0x04, 'M', 'Q', 'T', 'T',
		0x04, 0x02, 0x00, 0x0a,
		0x00, 0x03, 's', 'm', 'k',
	}
	if err := writeAll(conn, connect); err != nil {
		return err
	}
	header, body, err := readMQTTFrame(conn)
	if err != nil {
		return err
	}
	if header != 0x20 || !bytes.Equal(body, []byte{0x00, 0x00}) {
		return fmt.Errorf("mqtt connack header=%x body=%v", header, body)
	}
	if err := writeAll(conn, []byte{0xc0, 0x00}); err != nil {
		return err
	}
	header, body, err = readMQTTFrame(conn)
	if err != nil {
		return err
	}
	if header != 0xd0 || len(body) != 0 {
		return fmt.Errorf("mqtt pingresp header=%x body=%v", header, body)
	}
	publishBody := make([]byte, 0, 2+1+len(payload))
	publishBody = append(publishBody, 0x00, 0x01, 'x')
	publishBody = append(publishBody, payload...)
	if err := writeAll(conn, appendMQTTFrame(nil, 0x30, publishBody)); err != nil {
		return err
	}
	header, body, err = readMQTTFrame(conn)
	if err != nil {
		return err
	}
	if header != 0x30 || !bytes.Equal(body, publishBody) {
		return fmt.Errorf("mqtt publish echo header=%x body=%v", header, body)
	}
	return writeAll(conn, []byte{0xe0, 0x00})
}

func exchangeRedis(conn net.Conn) error {
	if err := writeAll(conn, []byte("*1\r\n$4\r\nPING\r\n*1\r\n$4\r\nPING\r\n")); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		line, err := readLine(conn)
		if err != nil {
			return err
		}
		if string(line) != "+PONG" {
			return fmt.Errorf("redis response=%q, want +PONG", line)
		}
	}
	return nil
}

func readHTTPResponse(r io.Reader) (string, map[string]string, []byte, error) {
	header := make([]byte, 0, 256)
	var one [1]byte
	for {
		if _, err := io.ReadFull(r, one[:]); err != nil {
			return "", nil, nil, err
		}
		header = append(header, one[0])
		if len(header) >= 4 && bytes.Equal(header[len(header)-4:], []byte("\r\n\r\n")) {
			break
		}
		if len(header) > 64*1024 {
			return "", nil, nil, fmt.Errorf("http header too large")
		}
	}
	lines := strings.Split(string(header[:len(header)-4]), "\r\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", nil, nil, fmt.Errorf("empty http response")
	}
	headers := make(map[string]string, len(lines)-1)
	for _, line := range lines[1:] {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return "", nil, nil, fmt.Errorf("invalid http header %q", line)
		}
		headers[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(value)
	}
	length := 0
	if value := headers["content-length"]; value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return "", nil, nil, fmt.Errorf("invalid content-length %q", value)
		}
		length = n
	}
	body := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, body); err != nil {
			return "", nil, nil, err
		}
	}
	return lines[0], headers, body, nil
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

func readMQTTFrame(r io.Reader) (byte, []byte, error) {
	var first [1]byte
	if _, err := io.ReadFull(r, first[:]); err != nil {
		return 0, nil, err
	}
	header := first[0]
	remaining := 0
	multiplier := 1
	for i := 0; i < 4; i++ {
		if _, err := io.ReadFull(r, first[:]); err != nil {
			return 0, nil, err
		}
		remaining += int(first[0]&127) * multiplier
		if first[0]&128 == 0 {
			body := make([]byte, remaining)
			if remaining > 0 {
				if _, err := io.ReadFull(r, body); err != nil {
					return 0, nil, err
				}
			}
			return header, body, nil
		}
		multiplier *= 128
	}
	return 0, nil, fmt.Errorf("invalid mqtt remaining length")
}

func appendMQTTFrame(dst []byte, header byte, payload []byte) []byte {
	dst = append(dst, header)
	remaining := len(payload)
	for {
		encoded := byte(remaining % 128)
		remaining /= 128
		if remaining > 0 {
			encoded |= 128
		}
		dst = append(dst, encoded)
		if remaining == 0 {
			return append(dst, payload...)
		}
	}
}

func appendLine(payload []byte) []byte {
	out := make([]byte, 0, len(payload)+1)
	out = append(out, payload...)
	out = append(out, '\n')
	return out
}

func readLine(r io.Reader) ([]byte, error) {
	out := make([]byte, 0, 64)
	var one [1]byte
	for {
		if _, err := io.ReadFull(r, one[:]); err != nil {
			return nil, err
		}
		if one[0] == '\n' {
			if len(out) > 0 && out[len(out)-1] == '\r' {
				out = out[:len(out)-1]
			}
			return out, nil
		}
		out = append(out, one[0])
	}
}

func writeAll(w io.Writer, src []byte) error {
	for len(src) > 0 {
		n, err := w.Write(src)
		if n > 0 {
			src = src[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

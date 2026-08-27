package smokeclient

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

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

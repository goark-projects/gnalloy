package benchhttp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func readResponse(reader *bufio.Reader, reply []byte, expected []byte) error {
	status, err := reader.ReadSlice('\n')
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(status, []byte("HTTP/1.1 200")) {
		return fmt.Errorf("benchhttp: unexpected status %q", strings.TrimSpace(string(status)))
	}
	contentLength := -1
	for {
		line, err := reader.ReadSlice('\n')
		if err != nil {
			return err
		}
		if bytes.Equal(line, []byte("\r\n")) {
			break
		}
		if value, ok := headerValue(line, "Content-Length"); ok {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return err
			}
			contentLength = n
		}
	}
	if contentLength != len(expected) {
		return fmt.Errorf("benchhttp: content length %d, want %d", contentLength, len(expected))
	}
	if len(reply) < contentLength {
		return fmt.Errorf("benchhttp: reply buffer too small")
	}
	if _, err := io.ReadFull(reader, reply[:contentLength]); err != nil {
		return err
	}
	if !bytes.Equal(reply[:contentLength], expected) {
		return fmt.Errorf("benchhttp: response body mismatch")
	}
	return nil
}

func headerValue(line []byte, name string) (string, bool) {
	key, value, ok := bytes.Cut(line, []byte(":"))
	if !ok || !strings.EqualFold(strings.TrimSpace(string(key)), name) {
		return "", false
	}
	return strings.TrimSpace(string(value)), true
}

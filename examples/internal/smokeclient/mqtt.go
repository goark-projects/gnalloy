package smokeclient

import (
	"bytes"
	"fmt"
	"io"
	"net"
)

func exchangeMQTT(conn net.Conn, payload []byte) error {
	return runMQTTClientSession(conn, payload, 1)
}

func runMQTTClientSession(conn net.Conn, payload []byte, count int) error {
	if err := openMQTTClientSession(conn); err != nil {
		return err
	}
	if err := pingMQTT(conn); err != nil {
		return err
	}
	publishBody := appendMQTTPublishBody(nil, payload)
	publishFrame := appendMQTTFrame(make([]byte, 0, 5+len(publishBody)), 0x30, publishBody)
	for i := 0; i < count; i++ {
		if err := exchangeMQTTPublish(conn, publishFrame, publishBody); err != nil {
			return fmt.Errorf("exchange %d: %w", i+1, err)
		}
	}
	return writeAll(conn, []byte{0xe0, 0x00})
}

func openMQTTClientSession(conn net.Conn) error {
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
	return nil
}

func pingMQTT(conn net.Conn) error {
	if err := writeAll(conn, []byte{0xc0, 0x00}); err != nil {
		return err
	}
	header, body, err := readMQTTFrame(conn)
	if err != nil {
		return err
	}
	if header != 0xd0 || len(body) != 0 {
		return fmt.Errorf("mqtt pingresp header=%x body=%v", header, body)
	}
	return nil
}

func exchangeMQTTPublish(conn net.Conn, publishFrame, publishBody []byte) error {
	if err := writeAll(conn, publishFrame); err != nil {
		return err
	}
	header, body, err := readMQTTFrame(conn)
	if err != nil {
		return err
	}
	if header != 0x30 || !bytes.Equal(body, publishBody) {
		return fmt.Errorf("mqtt publish echo header=%x body=%v", header, body)
	}
	return nil
}

func appendMQTTPublishBody(dst, payload []byte) []byte {
	dst = append(dst, 0x00, 0x01, 'x')
	return append(dst, payload...)
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

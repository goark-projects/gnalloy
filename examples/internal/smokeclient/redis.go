package smokeclient

import (
	"fmt"
	"net"
)

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

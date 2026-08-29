package main

import (
	"io"
	"net"
)

func estimateLatencySampleCount(connections int, messages int, rate int) int {
	if connections <= 0 || messages <= 0 || rate <= 0 {
		return 0
	}
	perConnection := messages / rate
	if messages%rate != 0 {
		perConnection++
	}
	return connections * perConnection
}

func resolveListenAddress(network string, addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	if port != "0" {
		return addr, nil
	}
	if network == "udp" {
		conn, err := net.ListenPacket("udp", net.JoinHostPort(host, port))
		if err != nil {
			return "", err
		}
		actual := conn.LocalAddr().String()
		if err := conn.Close(); err != nil {
			return "", err
		}
		return actual, nil
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return "", err
	}
	actual := ln.Addr().String()
	if err := ln.Close(); err != nil {
		return "", err
	}
	return actual, nil
}

func makePayload(size int, clientID int) []byte {
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(clientID + i)
	}
	return payload
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

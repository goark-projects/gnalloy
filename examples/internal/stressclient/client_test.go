package stressclient

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestRunRawLongScenario(t *testing.T) {
	addr, closeServer := startEchoServer(t, ProtocolRaw)
	defer closeServer()

	result, err := Run(nil, Config{
		Addr:            addr,
		Protocol:        ProtocolRaw,
		Scenario:        ScenarioLong,
		Connections:     4,
		MessagesPerConn: 8,
		PayloadSize:     32,
		Timeout:         3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Errors != 0 || result.TotalRequests != 32 {
		t.Fatalf("result=%+v", result)
	}
}

func TestRunLengthFieldHalfFrameScenario(t *testing.T) {
	addr, closeServer := startEchoServer(t, ProtocolLengthField)
	defer closeServer()

	result, err := Run(nil, Config{
		Addr:            addr,
		Protocol:        ProtocolLengthField,
		Scenario:        ScenarioHalfFrame,
		Connections:     3,
		MessagesPerConn: 5,
		PayloadSize:     24,
		Timeout:         3 * time.Second,
		Delay:           time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Errors != 0 || result.TotalRequests != 15 {
		t.Fatalf("result=%+v", result)
	}
}

func TestConfigRejectsInvalidScenario(t *testing.T) {
	_, err := Run(nil, Config{
		Addr:            "127.0.0.1:1",
		Protocol:        ProtocolRaw,
		Scenario:        "bad",
		Connections:     1,
		MessagesPerConn: 1,
		PayloadSize:     1,
	})
	if !errors.Is(err, ErrInvalidScenario) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidScenario)
	}
}

func startEchoServer(t *testing.T, protocol Protocol) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveEcho(conn, protocol)
		}
	}()
	return ln.Addr().String(), func() {
		_ = ln.Close()
		<-done
	}
}

func serveEcho(conn net.Conn, protocol Protocol) {
	defer conn.Close()
	switch protocol {
	case ProtocolRaw:
		_, _ = io.Copy(conn, conn)
	case ProtocolLengthField:
		for {
			var header [4]byte
			if _, err := io.ReadFull(conn, header[:]); err != nil {
				return
			}
			size := int(binary.BigEndian.Uint32(header[:]))
			payload := make([]byte, size)
			if _, err := io.ReadFull(conn, payload); err != nil {
				return
			}
			_ = writeAll(conn, header[:])
			_ = writeAll(conn, payload)
		}
	}
}

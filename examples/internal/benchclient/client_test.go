package benchclient

import (
	"errors"
	"io"
	"net"
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

	payload := []byte("ping")
	reply := make([]byte, len(payload))
	if err := exchange(client, ProtocolRaw, payload, reply, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeLengthField(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		var payload [4]byte
		size, err := readFramePayload(server, payload[:])
		if err != nil {
			return
		}
		var frame [8]byte
		frame[3] = byte(size)
		copy(frame[4:], payload[:size])
		_, _ = server.Write(frame[:4+size])
	}()

	payload := []byte("ping")
	frame := make([]byte, 4+len(payload))
	reply := make([]byte, len(payload))
	if err := exchange(client, ProtocolLengthField, payload, nil, frame, reply); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeLine(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		var payload [4]byte
		size, err := readLinePayload(server, payload[:])
		if err != nil {
			return
		}
		_, _ = server.Write(append(payload[:size], '\n'))
	}()

	payload := []byte("ping")
	frame := make([]byte, len(payload)+1)
	reply := make([]byte, len(payload))
	if err := exchange(client, ProtocolLine, payload, nil, frame, reply); err != nil {
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

	payload := []byte("ping")
	reply := make([]byte, len(payload))
	if err := exchange(client, ProtocolFixed, payload, reply, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestConfigValidate(t *testing.T) {
	cfg := Config{Addr: "127.0.0.1:1", Protocol: ProtocolRaw, Connections: 1, MessagesPerConn: 1, PayloadSize: 1}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	cfg.Protocol = Protocol("bad")
	if err := cfg.validate(); !errors.Is(err, ErrInvalidProtocol) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidProtocol)
	}
}

func TestSummarize(t *testing.T) {
	got := summarize([]int64{300, 100, 200}, 3, 1, time.Second)
	if got.TotalRequests != 3 || got.Errors != 1 {
		t.Fatalf("result=%+v", got)
	}
	if got.P50 != 200 || got.P95 != 300 || got.P99 != 300 || got.Max != 300 {
		t.Fatalf("percentiles=%s/%s/%s max=%s", got.P50, got.P95, got.P99, got.Max)
	}
	if got.Throughput != 3 {
		t.Fatalf("throughput=%f, want 3", got.Throughput)
	}
}

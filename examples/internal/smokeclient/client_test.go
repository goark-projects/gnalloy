package smokeclient

import (
	"errors"
	"io"
	"net"
	"testing"
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

	if err := exchange(client, ProtocolRaw, []byte("ping")); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeLengthField(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		payload, err := readFrame(server)
		if err != nil {
			return
		}
		_ = writeFrame(server, payload)
	}()

	if err := exchange(client, ProtocolLengthField, []byte("ping")); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeRejectsInvalidProtocol(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	err := exchange(client, Protocol("bad"), []byte("ping"))
	if !errors.Is(err, ErrInvalidProtocol) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidProtocol)
	}
}

package smokeclient

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

type Protocol string

const (
	ProtocolRaw         Protocol = "raw"
	ProtocolLengthField Protocol = "length-field"
	ProtocolUDP         Protocol = "udp"
	ProtocolLine        Protocol = "line"
	ProtocolFixed       Protocol = "fixed"
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
	default:
		return ErrInvalidProtocol
	}
}

func validateProtocol(protocol Protocol) error {
	switch protocol {
	case ProtocolRaw, ProtocolLengthField, ProtocolUDP, ProtocolLine, ProtocolFixed:
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

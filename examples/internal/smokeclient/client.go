package smokeclient

import (
	"context"
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

	return runExchanges(conn, cfg.Protocol, cfg.Message, cfg.Count)
}

func runExchanges(conn net.Conn, protocol Protocol, payload []byte, count int) error {
	switch protocol {
	case ProtocolWebSocket:
		return runWebSocketClientSession(conn, payload, count)
	case ProtocolMQTT:
		return runMQTTClientSession(conn, payload, count)
	}
	for i := 0; i < count; i++ {
		if err := exchange(conn, protocol, payload); err != nil {
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

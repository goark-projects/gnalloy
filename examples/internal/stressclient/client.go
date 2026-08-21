package stressclient

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Protocol string

const (
	ProtocolRaw         Protocol = "raw"
	ProtocolLengthField Protocol = "length-field"
)

type Scenario string

const (
	ScenarioLong      Scenario = "long"
	ScenarioShort     Scenario = "short"
	ScenarioHalfFrame Scenario = "half-frame"
	ScenarioSlow      Scenario = "slow"
	ScenarioMixed     Scenario = "mixed"
)

var (
	ErrInvalidProtocol = errors.New("gnalloy/examples: invalid stress protocol")
	ErrInvalidScenario = errors.New("gnalloy/examples: invalid stress scenario")
	ErrInvalidConfig   = errors.New("gnalloy/examples: invalid stress config")
)

type Config struct {
	Addr            string
	Protocol        Protocol
	Scenario        Scenario
	Connections     int
	MessagesPerConn int
	PayloadSize     int
	Timeout         time.Duration
	Delay           time.Duration
}

type Result struct {
	TotalRequests int64
	Errors        int64
	Elapsed       time.Duration
}

// Run 执行面向 TCP 生命周期的压力场景，覆盖长连接、短连接、半包与慢客户端。
func Run(ctx context.Context, cfg Config) (Result, error) {
	if err := cfg.validate(); err != nil {
		return Result{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	var total int64
	var failures int64
	for _, scenario := range expandScenarios(cfg.Scenario) {
		cfg.Scenario = scenario
		requests, errors := runScenario(ctx, cfg)
		total += requests
		failures += errors
		if ctx.Err() != nil {
			return Result{TotalRequests: total, Errors: failures, Elapsed: time.Since(start)}, ctx.Err()
		}
	}
	return Result{TotalRequests: total, Errors: failures, Elapsed: time.Since(start)}, nil
}

func (c Config) validate() error {
	if c.Addr == "" {
		return fmt.Errorf("%w: empty addr", ErrInvalidConfig)
	}
	if c.Connections <= 0 || c.MessagesPerConn <= 0 || c.PayloadSize <= 0 {
		return fmt.Errorf("%w: connections, messages and payload-size must be positive", ErrInvalidConfig)
	}
	switch c.Protocol {
	case ProtocolRaw, ProtocolLengthField:
	default:
		return ErrInvalidProtocol
	}
	switch c.Scenario {
	case ScenarioLong, ScenarioShort, ScenarioHalfFrame, ScenarioSlow, ScenarioMixed:
	default:
		return ErrInvalidScenario
	}
	return nil
}

func expandScenarios(scenario Scenario) []Scenario {
	if scenario != ScenarioMixed {
		return []Scenario{scenario}
	}
	return []Scenario{ScenarioLong, ScenarioShort, ScenarioHalfFrame, ScenarioSlow}
}

func runScenario(ctx context.Context, cfg Config) (int64, int64) {
	var requests atomic.Int64
	var failures atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < cfg.Connections; i++ {
		clientID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runWorker(ctx, cfg, clientID, &requests); err != nil {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	return requests.Load(), failures.Load()
}

func runWorker(ctx context.Context, cfg Config, clientID int, requests *atomic.Int64) error {
	switch cfg.Scenario {
	case ScenarioLong:
		return runLong(ctx, cfg, clientID, requests)
	case ScenarioShort:
		return runShort(ctx, cfg, clientID, requests)
	case ScenarioHalfFrame:
		return runLongWithMode(ctx, cfg, clientID, requests, writeHalfFrame)
	case ScenarioSlow:
		return runLongWithMode(ctx, cfg, clientID, requests, writeSlow)
	default:
		return ErrInvalidScenario
	}
}

func runLong(ctx context.Context, cfg Config, clientID int, requests *atomic.Int64) error {
	return runLongWithMode(ctx, cfg, clientID, requests, writeFull)
}

func runLongWithMode(ctx context.Context, cfg Config, clientID int, requests *atomic.Int64, mode writeMode) error {
	conn, err := dial(ctx, cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	payload := makePayload(cfg.PayloadSize, clientID)
	reply := make([]byte, cfg.PayloadSize)
	frame := make([]byte, 4+cfg.PayloadSize)
	for i := 0; i < cfg.MessagesPerConn; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		payload[0] = byte(clientID + i)
		if err := exchange(conn, cfg, payload, reply, frame, mode); err != nil {
			return err
		}
		requests.Add(1)
	}
	return nil
}

func runShort(ctx context.Context, cfg Config, clientID int, requests *atomic.Int64) error {
	payload := makePayload(cfg.PayloadSize, clientID)
	reply := make([]byte, cfg.PayloadSize)
	frame := make([]byte, 4+cfg.PayloadSize)
	for i := 0; i < cfg.MessagesPerConn; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		conn, err := dial(ctx, cfg)
		if err != nil {
			return err
		}
		payload[0] = byte(clientID + i)
		err = exchange(conn, cfg, payload, reply, frame, writeFull)
		closeErr := conn.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		requests.Add(1)
	}
	return nil
}

func dial(ctx context.Context, cfg Config) (net.Conn, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", cfg.Addr)
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

type writeMode uint8

const (
	writeFull writeMode = iota
	writeHalfFrame
	writeSlow
)

func exchange(conn net.Conn, cfg Config, payload []byte, reply []byte, frame []byte, mode writeMode) error {
	switch cfg.Protocol {
	case ProtocolRaw:
		if err := writePayload(conn, payload, cfg.Delay, mode); err != nil {
			return err
		}
		if _, err := io.ReadFull(conn, reply[:len(payload)]); err != nil {
			return err
		}
		if string(reply[:len(payload)]) != string(payload) {
			return fmt.Errorf("raw echo mismatch")
		}
		return nil
	case ProtocolLengthField:
		if len(frame) < 4+len(payload) {
			return ErrInvalidConfig
		}
		binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
		copy(frame[4:], payload)
		if err := writePayload(conn, frame[:4+len(payload)], cfg.Delay, mode); err != nil {
			return err
		}
		size, err := readFramePayload(conn, reply)
		if err != nil {
			return err
		}
		if size != len(payload) || string(reply[:size]) != string(payload) {
			return fmt.Errorf("length-field echo mismatch")
		}
		return nil
	default:
		return ErrInvalidProtocol
	}
}

func writePayload(w io.Writer, src []byte, delay time.Duration, mode writeMode) error {
	if delay <= 0 {
		delay = time.Millisecond
	}
	switch mode {
	case writeFull:
		return writeAll(w, src)
	case writeHalfFrame:
		split := len(src) / 2
		if split <= 0 {
			split = 1
		}
		if err := writeAll(w, src[:split]); err != nil {
			return err
		}
		time.Sleep(delay)
		return writeAll(w, src[split:])
	case writeSlow:
		for len(src) > 0 {
			if err := writeAll(w, src[:1]); err != nil {
				return err
			}
			src = src[1:]
			time.Sleep(delay)
		}
		return nil
	default:
		return ErrInvalidScenario
	}
}

func readFramePayload(r io.Reader, dst []byte) (int, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, err
	}
	size := int(binary.BigEndian.Uint32(header[:]))
	if size > len(dst) {
		return 0, fmt.Errorf("frame too large: %d", size)
	}
	if _, err := io.ReadFull(r, dst[:size]); err != nil {
		return 0, err
	}
	return size, nil
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

func makePayload(size int, clientID int) []byte {
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(clientID + i)
	}
	return payload
}

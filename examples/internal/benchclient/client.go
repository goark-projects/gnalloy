package benchclient

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Protocol string

const (
	ProtocolRaw         Protocol = "raw"
	ProtocolLengthField Protocol = "length-field"
)

var (
	ErrInvalidProtocol = errors.New("gnalloy/examples: invalid bench protocol")
	ErrInvalidConfig   = errors.New("gnalloy/examples: invalid bench config")
)

type Config struct {
	Addr            string
	Protocol        Protocol
	Connections     int
	MessagesPerConn int
	PayloadSize     int
	Timeout         time.Duration
}

type Result struct {
	TotalRequests int64
	Errors        int64
	Elapsed       time.Duration
	Throughput    float64

	Avg time.Duration
	P50 time.Duration
	P95 time.Duration
	P99 time.Duration
	Max time.Duration
}

// Run 执行固定连接数、固定消息数的 TCP echo 压测。
func Run(ctx context.Context, cfg Config) (Result, error) {
	if err := cfg.validate(); err != nil {
		return Result{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	total := cfg.Connections * cfg.MessagesPerConn
	latencies := make([]int64, total)
	var (
		nextSample atomic.Int64
		errors     atomic.Int64
		wg         sync.WaitGroup
	)

	start := time.Now()
	for i := 0; i < cfg.Connections; i++ {
		clientID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runConnection(ctx, cfg, clientID, &nextSample, latencies); err != nil {
				errors.Add(1)
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	successes := nextSample.Load()
	return summarize(latencies[:successes], successes, errors.Load(), elapsed), nil
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
	return nil
}

func runConnection(ctx context.Context, cfg Config, clientID int, nextSample *atomic.Int64, latencies []int64) error {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", cfg.Addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}

	payload := makePayload(cfg.PayloadSize, clientID)
	reply := make([]byte, cfg.PayloadSize)
	frame := make([]byte, 4+cfg.PayloadSize)
	frameReply := make([]byte, cfg.PayloadSize)
	for i := 0; i < cfg.MessagesPerConn; i++ {
		payload[0] = byte(clientID + i)
		start := time.Now()
		if err := exchange(conn, cfg.Protocol, payload, reply, frame, frameReply); err != nil {
			return err
		}
		idx := nextSample.Add(1) - 1
		if idx >= 0 && int(idx) < len(latencies) {
			latencies[idx] = time.Since(start).Nanoseconds()
		}
	}
	return nil
}

func exchange(conn net.Conn, protocol Protocol, payload []byte, rawReply []byte, frame []byte, frameReply []byte) error {
	switch protocol {
	case ProtocolRaw:
		if err := writeAll(conn, payload); err != nil {
			return err
		}
		if _, err := io.ReadFull(conn, rawReply[:len(payload)]); err != nil {
			return err
		}
		if string(rawReply[:len(payload)]) != string(payload) {
			return fmt.Errorf("raw echo mismatch")
		}
		return nil
	case ProtocolLengthField:
		if len(frame) < 4+len(payload) || len(frameReply) < len(payload) {
			return ErrInvalidConfig
		}
		binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
		copy(frame[4:], payload)
		if err := writeAll(conn, frame[:4+len(payload)]); err != nil {
			return err
		}
		size, err := readFramePayload(conn, frameReply)
		if err != nil {
			return err
		}
		if size != len(payload) || string(frameReply[:size]) != string(payload) {
			return fmt.Errorf("length-field echo mismatch")
		}
		return nil
	default:
		return ErrInvalidProtocol
	}
}

func summarize(latencies []int64, total int64, errors int64, elapsed time.Duration) Result {
	result := Result{
		TotalRequests: total,
		Errors:        errors,
		Elapsed:       elapsed,
	}
	if elapsed > 0 {
		result.Throughput = float64(total) / elapsed.Seconds()
	}
	if len(latencies) == 0 {
		return result
	}

	sort.Slice(latencies, func(i int, j int) bool { return latencies[i] < latencies[j] })
	var sum int64
	for _, v := range latencies {
		sum += v
	}
	result.Avg = time.Duration(sum / int64(len(latencies)))
	result.P50 = percentile(latencies, 50)
	result.P95 = percentile(latencies, 95)
	result.P99 = percentile(latencies, 99)
	result.Max = time.Duration(latencies[len(latencies)-1])
	return result
}

func percentile(values []int64, p int) time.Duration {
	if len(values) == 0 {
		return 0
	}
	idx := (len(values)*p + 99) / 100
	if idx <= 0 {
		idx = 1
	}
	if idx > len(values) {
		idx = len(values)
	}
	return time.Duration(values[idx-1])
}

func makePayload(size int, clientID int) []byte {
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(clientID + i)
	}
	return payload
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

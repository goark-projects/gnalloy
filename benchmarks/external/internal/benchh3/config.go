package benchh3

import (
	"crypto/tls"
	"fmt"
	"strings"
	"time"
)

const alpnHTTP3 = "h3"

// Config 描述 HTTP/3 over QUIC 负载。
type Config struct {
	Addr              string
	ServerName        string
	Payload           int
	Connections       int
	Messages          int
	Timeout           time.Duration
	LatencySampleRate int
	WarmupMessages    int
	TLS               *tls.Config
}

// Result 是一次 HTTP/3 负载的汇总指标。
type Result struct {
	TotalRequests      int64
	Errors             int64
	Elapsed            time.Duration
	Throughput         float64
	NsPerOp            float64
	NegotiatedProtocol string
	Latency            LatencySummary
}

// LatencySummary 汇总采样到的单 stream 往返延迟。
type LatencySummary struct {
	Samples int64
	P50     time.Duration
	P95     time.Duration
	P99     time.Duration
	P999    time.Duration
	Max     time.Duration
}

// Validate 校验 HTTP/3 客户端压测边界。
func (c Config) Validate() error {
	if strings.TrimSpace(c.Addr) == "" {
		return fmt.Errorf("benchh3: empty addr")
	}
	if c.Payload <= 0 || c.Connections <= 0 || c.Messages <= 0 {
		return fmt.Errorf("benchh3: payload, connections and messages must be positive")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("benchh3: timeout must be positive")
	}
	if c.LatencySampleRate < 0 {
		return fmt.Errorf("benchh3: latency-sample-rate must not be negative")
	}
	if c.WarmupMessages < 0 {
		return fmt.Errorf("benchh3: warmup-messages must not be negative")
	}
	return nil
}

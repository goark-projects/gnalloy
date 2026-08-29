package benchh2

import (
	"crypto/tls"
	"fmt"
	"strings"
	"time"
)

// Config 描述 HTTP/2 prior-knowledge 或 TLS ALPN 负载。
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

// Result 是一次 HTTP/2 负载的汇总指标。
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

// Validate 校验 HTTP/2 客户端压测边界。
func (c Config) Validate() error {
	if strings.TrimSpace(c.Addr) == "" {
		return fmt.Errorf("benchh2: empty addr")
	}
	if c.Payload <= 0 || c.Connections <= 0 || c.Messages <= 0 {
		return fmt.Errorf("benchh2: payload, connections and messages must be positive")
	}
	if c.Messages > maxClientStreamCount-c.WarmupMessages {
		return fmt.Errorf("benchh2: messages exceed stream id budget")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("benchh2: timeout must be positive")
	}
	if c.LatencySampleRate < 0 {
		return fmt.Errorf("benchh2: latency-sample-rate must not be negative")
	}
	if c.WarmupMessages < 0 {
		return fmt.Errorf("benchh2: warmup-messages must not be negative")
	}
	return nil
}

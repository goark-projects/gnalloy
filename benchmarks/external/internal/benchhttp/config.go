package benchhttp

import (
	"crypto/tls"
	"fmt"
	"strings"
	"time"
)

// Config 描述原始 HTTP/1.1 keep-alive 客户端负载。
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

// Result 是一次 HTTP/1.1 负载的汇总指标。
type Result struct {
	TotalRequests      int64
	Errors             int64
	Elapsed            time.Duration
	Throughput         float64
	NsPerOp            float64
	NegotiatedProtocol string
	Latency            LatencySummary
}

// LatencySummary 汇总采样到的单请求往返延迟。
type LatencySummary struct {
	Samples int64
	P50     time.Duration
	P95     time.Duration
	P99     time.Duration
	P999    time.Duration
	Max     time.Duration
}

// Validate 校验 HTTP 客户端压测边界。
func (c Config) Validate() error {
	if strings.TrimSpace(c.Addr) == "" {
		return fmt.Errorf("benchhttp: empty addr")
	}
	if c.Payload <= 0 || c.Connections <= 0 || c.Messages <= 0 {
		return fmt.Errorf("benchhttp: payload, connections and messages must be positive")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("benchhttp: timeout must be positive")
	}
	if c.LatencySampleRate < 0 {
		return fmt.Errorf("benchhttp: latency-sample-rate must not be negative")
	}
	if c.WarmupMessages < 0 {
		return fmt.Errorf("benchhttp: warmup-messages must not be negative")
	}
	return nil
}

package parity

import (
	"strconv"
	"strings"
)

// BenchmarkMetric 是从 Go benchmark 输出中抽取的结构化指标。
type BenchmarkMetric struct {
	Name        string  `json:"name"`
	Iterations  int64   `json:"iterations"`
	NsPerOp     float64 `json:"nsPerOp,omitempty"`
	BytesPerOp  int64   `json:"bytesPerOp,omitempty"`
	AllocsPerOp int64   `json:"allocsPerOp,omitempty"`
	Raw         string  `json:"raw"`
}

// ParseBenchmarkMetrics 解析 `go test -bench` 的标准输出。
func ParseBenchmarkMetrics(output string) []BenchmarkMetric {
	lines := strings.Split(output, "\n")
	metrics := make([]BenchmarkMetric, 0)
	for _, line := range lines {
		metric, ok := parseBenchmarkLine(strings.TrimSpace(line))
		if ok {
			metrics = append(metrics, metric)
		}
	}
	return metrics
}

func parseBenchmarkLine(line string) (BenchmarkMetric, bool) {
	if line == "" || !strings.HasPrefix(line, "Benchmark") {
		return BenchmarkMetric{}, false
	}
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return BenchmarkMetric{}, false
	}
	iterations, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return BenchmarkMetric{}, false
	}
	metric := BenchmarkMetric{Name: fields[0], Iterations: iterations, Raw: line}
	for i := 2; i < len(fields); i++ {
		switch fields[i] {
		case "ns/op":
			metric.NsPerOp = parseFloatBefore(fields, i)
		case "B/op":
			metric.BytesPerOp = parseIntBefore(fields, i)
		case "allocs/op":
			metric.AllocsPerOp = parseIntBefore(fields, i)
		}
	}
	return metric, true
}

func parseFloatBefore(fields []string, i int) float64 {
	if i == 0 {
		return 0
	}
	value, _ := strconv.ParseFloat(fields[i-1], 64)
	return value
}

func parseIntBefore(fields []string, i int) int64 {
	if i == 0 {
		return 0
	}
	value, _ := strconv.ParseInt(fields[i-1], 10, 64)
	return value
}

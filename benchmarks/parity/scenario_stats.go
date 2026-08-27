package parity

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ScenarioStats 是 external harness 汇总行的结构化指标。
type ScenarioStats struct {
	Framework           string        `json:"framework,omitempty"`
	Protocol            string        `json:"protocol,omitempty"`
	Backend             string        `json:"backend,omitempty"`
	PayloadBytes        int64         `json:"payloadBytes"`
	Connections         int64         `json:"connections"`
	Messages            int64         `json:"messages"`
	TotalRequests       int64         `json:"totalRequests"`
	Errors              int64         `json:"errors"`
	Elapsed             time.Duration `json:"elapsed"`
	ThroughputOpsPerSec float64       `json:"throughputOpsPerSec"`
	Raw                 string        `json:"raw"`
}

// ParseScenarioStats 解析 harness 输出中的 key=value 汇总行。
func ParseScenarioStats(output string) []ScenarioStats {
	lines := strings.Split(output, "\n")
	stats := make([]ScenarioStats, 0)
	for _, line := range lines {
		stat, ok := parseScenarioStatsLine(strings.TrimSpace(line))
		if ok {
			stats = append(stats, stat)
		}
	}
	return stats
}

func parseScenarioStatsLine(line string) (ScenarioStats, bool) {
	if line == "" || !strings.Contains(line, "framework=") {
		return ScenarioStats{}, false
	}
	fields := strings.Fields(line)
	stat := ScenarioStats{Raw: line}
	seenFramework := false
	seenProtocol := false
	for i := 0; i < len(fields); i++ {
		key, value, ok := strings.Cut(fields[i], "=")
		if !ok {
			continue
		}
		switch key {
		case "framework":
			stat.Framework = value
			seenFramework = true
		case "protocol":
			stat.Protocol = value
			seenProtocol = true
		case "backend":
			stat.Backend = value
		case "payload":
			stat.PayloadBytes = parseIntMetric(value)
		case "connections":
			stat.Connections = parseIntMetric(value)
		case "messages":
			stat.Messages = parseIntMetric(value)
		case "total":
			stat.TotalRequests = parseIntMetric(value)
		case "errors":
			stat.Errors = parseIntMetric(value)
		case "elapsed":
			stat.Elapsed = parseDurationMetric(value)
		case "throughput":
			stat.ThroughputOpsPerSec, _ = strconv.ParseFloat(value, 64)
		}
	}
	if !seenFramework || !seenProtocol {
		return ScenarioStats{}, false
	}
	return stat, true
}

func parseIntMetric(value string) int64 {
	value = strings.TrimSuffix(strings.TrimSpace(value), "B")
	n, _ := strconv.ParseInt(value, 10, 64)
	return n
}

func parseDurationMetric(value string) time.Duration {
	if value == "" {
		return 0
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}
	duration, _ := parseJavaDuration(value)
	return duration
}

func parseJavaDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "PT") {
		return 0, fmt.Errorf("%w: %s", ErrInvalidScenario, value)
	}
	rest := strings.TrimPrefix(value, "PT")
	if rest == "" {
		return 0, fmt.Errorf("%w: empty java duration", ErrInvalidScenario)
	}
	var total time.Duration
	for len(rest) > 0 {
		idx := strings.IndexAny(rest, "HMS")
		if idx <= 0 {
			return 0, fmt.Errorf("%w: %s", ErrInvalidScenario, value)
		}
		number := rest[:idx]
		unit := rest[idx]
		part, err := strconv.ParseFloat(number, 64)
		if err != nil {
			return 0, err
		}
		switch unit {
		case 'H':
			total += time.Duration(part * float64(time.Hour))
		case 'M':
			total += time.Duration(part * float64(time.Minute))
		case 'S':
			total += time.Duration(part * float64(time.Second))
		default:
			return 0, fmt.Errorf("%w: %s", ErrInvalidScenario, value)
		}
		rest = rest[idx+1:]
	}
	return total, nil
}

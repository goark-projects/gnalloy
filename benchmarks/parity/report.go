package parity

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Format 描述报告输出格式。
type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
)

// WriteReport 写出指定格式报告。
func WriteReport(w io.Writer, report Report, format Format) error {
	if w == nil {
		return fmt.Errorf("%w: nil writer", ErrInvalidFormat)
	}
	switch normalizeFormat(format) {
	case FormatMarkdown:
		return writeMarkdown(w, report)
	case FormatJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	default:
		return ErrInvalidFormat
	}
}

func normalizeFormat(format Format) Format {
	switch strings.ToLower(strings.TrimSpace(string(format))) {
	case "", "md", "markdown":
		return FormatMarkdown
	case "json":
		return FormatJSON
	default:
		return format
	}
}

func writeMarkdown(w io.Writer, report Report) error {
	var b strings.Builder
	title := strings.TrimSpace(report.Name)
	if title == "" {
		title = "gnalloy parity benchmark"
	}
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")
	if strings.TrimSpace(report.Notes) != "" {
		b.WriteString(report.Notes)
		b.WriteString("\n\n")
	}
	b.WriteString("## Machine\n\n")
	b.WriteString("| Field | Value |\n| --- | --- |\n")
	writeRow(&b, "hostname", report.Machine.Hostname)
	writeRow(&b, "os", report.Machine.OS)
	writeRow(&b, "arch", report.Machine.Arch)
	writeRow(&b, "cpus", strconv.Itoa(report.Machine.CPUs))
	writeRow(&b, "go", report.Machine.Go)
	writeRow(&b, "ips", report.Machine.IPs)
	writeSummary(&b, report)
	b.WriteString("\n## Scenarios\n\n")
	for _, result := range report.Scenarios {
		writeScenario(&b, result)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeSummary(b *strings.Builder, report Report) {
	if !hasStats(report) && !hasMetrics(report) {
		return
	}
	b.WriteString("\n## Summary\n\n")
	if hasStats(report) {
		writeStatsSummary(b, report)
		writeStatsAggregateSummary(b, report)
	}
	if hasMetrics(report) {
		writeBenchmarkSummary(b, report)
	}
}

func writeStatsSummary(b *strings.Builder, report Report) {
	b.WriteString("| Scenario | Framework | Protocol | ALPN | Backend | Loops | Total | Errors | Throughput ops/s | P99 latency ns | RSS bytes | GC count | Elapsed |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, result := range report.Scenarios {
		for _, stat := range result.Stats {
			writeStatsRow(b, result.Scenario, stat)
		}
	}
	b.WriteString("\n")
}

func writeBenchmarkSummary(b *strings.Builder, report Report) {
	b.WriteString("| Scenario | Framework | Protocol | Benchmark | ns/op | B/op | allocs/op |\n")
	b.WriteString("| --- | --- | --- | --- | ---: | ---: | ---: |\n")
	for _, result := range report.Scenarios {
		for _, metric := range result.Metrics {
			writeMetricRow(b, result.Scenario, metric)
		}
	}
	b.WriteString("\n")
}

func writeStatsAggregateSummary(b *strings.Builder, report Report) {
	summaries := statsSummaries(report)
	if len(summaries) == 0 {
		return
	}
	b.WriteString("| Scenario | Framework | Protocol | ALPN | Backend | Loops | Samples | Throughput min | Throughput median | Throughput max | Throughput mean | Median ns/op | Median P99 latency ns | Max RSS bytes | GC count | Errors |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, summary := range summaries {
		b.WriteString("| ")
		b.WriteString(escapeCell(summary.Scenario))
		b.WriteString(" | ")
		b.WriteString(escapeCell(summary.Framework))
		b.WriteString(" | ")
		b.WriteString(escapeCell(summary.Protocol))
		b.WriteString(" | ")
		b.WriteString(escapeCell(summary.NegotiatedProtocol))
		b.WriteString(" | ")
		b.WriteString(escapeCell(summary.Backend))
		b.WriteString(" | ")
		b.WriteString(escapeCell(summary.LoopSummary))
		b.WriteString(" | ")
		b.WriteString(strconv.Itoa(summary.Samples))
		b.WriteString(" | ")
		b.WriteString(formatFloat(summary.MinThroughputOps))
		b.WriteString(" | ")
		b.WriteString(formatFloat(summary.MedianThroughputOps))
		b.WriteString(" | ")
		b.WriteString(formatFloat(summary.MaxThroughputOps))
		b.WriteString(" | ")
		b.WriteString(formatFloat(summary.MeanThroughputOps))
		b.WriteString(" | ")
		b.WriteString(formatFloat(summary.MedianNsPerOp))
		b.WriteString(" | ")
		b.WriteString(formatFloat(summary.MedianP99LatencyNs))
		b.WriteString(" | ")
		b.WriteString(strconv.FormatInt(summary.MaxRSSBytes, 10))
		b.WriteString(" | ")
		b.WriteString(strconv.FormatInt(summary.TotalGCCount, 10))
		b.WriteString(" | ")
		b.WriteString(strconv.FormatInt(summary.TotalErrors, 10))
		b.WriteString(" |\n")
	}
	b.WriteString("\n")
}

func writeScenario(b *strings.Builder, result ScenarioResult) {
	scenario := result.Scenario
	b.WriteString("### ")
	b.WriteString(scenario.Name)
	b.WriteString("\n\n")
	b.WriteString("| Field | Value |\n| --- | --- |\n")
	writeRow(b, "framework", scenario.Framework)
	writeRow(b, "protocol", scenario.Protocol)
	writeRow(b, "backend", scenario.Backend)
	writeRow(b, "payload", scenario.Payload)
	if scenario.Warmup > 0 {
		writeRow(b, "warmup", strconv.Itoa(scenario.Warmup))
	}
	if scenarioRepeat(scenario) > 1 {
		writeRow(b, "repeat", strconv.Itoa(scenarioRepeat(scenario)))
	}
	writeRow(b, "duration", result.Duration.String())
	writeRow(b, "exitCode", strconv.Itoa(result.ExitCode))
	writeRow(b, "skipped", strconv.FormatBool(result.Skipped))
	if result.Error != "" {
		writeRow(b, "error", result.Error)
	}
	b.WriteString("\nCommand:\n\n```text\n")
	b.WriteString(escapeFence(strings.Join(scenario.Command, " ")))
	b.WriteString("\n```\n\n")
	if result.Output != "" {
		b.WriteString("Output:\n\n```text\n")
		b.WriteString(escapeFence(result.Output))
		b.WriteString("\n```\n\n")
	}
	if len(result.Samples) > 0 {
		b.WriteString("Samples:\n\n")
		b.WriteString("| Index | Exit | Duration | Stats | Metrics | Error |\n")
		b.WriteString("| ---: | ---: | ---: | ---: | ---: | --- |\n")
		for _, sample := range result.Samples {
			b.WriteString("| ")
			b.WriteString(strconv.Itoa(sample.Index))
			b.WriteString(" | ")
			b.WriteString(strconv.Itoa(sample.ExitCode))
			b.WriteString(" | ")
			b.WriteString(sample.Duration.String())
			b.WriteString(" | ")
			b.WriteString(strconv.Itoa(len(sample.Stats)))
			b.WriteString(" | ")
			b.WriteString(strconv.Itoa(len(sample.Metrics)))
			b.WriteString(" | ")
			b.WriteString(escapeCell(sample.Error))
			b.WriteString(" |\n")
		}
		b.WriteString("\n")
	}
	if len(result.Stats) > 0 {
		b.WriteString("Stats:\n\n")
		b.WriteString("| Framework | Protocol | ALPN | Backend | Loops | Payload | Connections | Messages | Total | Errors | Throughput ops/s | P50 latency ns | P99 latency ns | RSS bytes | GC count | Elapsed |\n")
		b.WriteString("| --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for _, stat := range result.Stats {
			b.WriteString("| ")
			b.WriteString(escapeCell(stat.Framework))
			b.WriteString(" | ")
			b.WriteString(escapeCell(stat.Protocol))
			b.WriteString(" | ")
			b.WriteString(escapeCell(stat.NegotiatedProtocol))
			b.WriteString(" | ")
			b.WriteString(escapeCell(stat.Backend))
			b.WriteString(" | ")
			b.WriteString(escapeCell(loopSummary(stat)))
			b.WriteString(" | ")
			b.WriteString(strconv.FormatInt(stat.PayloadBytes, 10))
			b.WriteString(" | ")
			b.WriteString(strconv.FormatInt(stat.Connections, 10))
			b.WriteString(" | ")
			b.WriteString(strconv.FormatInt(stat.Messages, 10))
			b.WriteString(" | ")
			b.WriteString(strconv.FormatInt(stat.TotalRequests, 10))
			b.WriteString(" | ")
			b.WriteString(strconv.FormatInt(stat.Errors, 10))
			b.WriteString(" | ")
			b.WriteString(formatFloat(stat.ThroughputOpsPerSec))
			b.WriteString(" | ")
			b.WriteString(strconv.FormatInt(stat.P50LatencyNanos, 10))
			b.WriteString(" | ")
			b.WriteString(strconv.FormatInt(stat.P99LatencyNanos, 10))
			b.WriteString(" | ")
			b.WriteString(strconv.FormatInt(stat.RSSBytes, 10))
			b.WriteString(" | ")
			b.WriteString(strconv.FormatInt(stat.GCCount, 10))
			b.WriteString(" | ")
			b.WriteString(stat.Elapsed.String())
			b.WriteString(" |\n")
		}
		b.WriteString("\n")
	}
	if len(result.Metrics) > 0 {
		b.WriteString("Metrics:\n\n")
		b.WriteString("| Benchmark | Iterations | ns/op | B/op | allocs/op |\n")
		b.WriteString("| --- | ---: | ---: | ---: | ---: |\n")
		for _, metric := range result.Metrics {
			b.WriteString("| ")
			b.WriteString(escapeCell(metric.Name))
			b.WriteString(" | ")
			b.WriteString(strconv.FormatInt(metric.Iterations, 10))
			b.WriteString(" | ")
			b.WriteString(formatFloat(metric.NsPerOp))
			b.WriteString(" | ")
			b.WriteString(strconv.FormatInt(metric.BytesPerOp, 10))
			b.WriteString(" | ")
			b.WriteString(strconv.FormatInt(metric.AllocsPerOp, 10))
			b.WriteString(" |\n")
		}
		b.WriteString("\n")
	}
}

func writeRow(b *strings.Builder, key string, value string) {
	if value == "" {
		value = "-"
	}
	b.WriteString("| ")
	b.WriteString(escapeCell(key))
	b.WriteString(" | ")
	b.WriteString(escapeCell(value))
	b.WriteString(" |\n")
}

func escapeCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "|", "\\|")
}

func escapeFence(value string) string {
	return strings.ReplaceAll(value, "```", "`\u200b``")
}

func hasMetrics(report Report) bool {
	for _, result := range report.Scenarios {
		if len(result.Metrics) > 0 {
			return true
		}
	}
	return false
}

func hasStats(report Report) bool {
	for _, result := range report.Scenarios {
		if len(result.Stats) > 0 {
			return true
		}
	}
	return false
}

func writeStatsRow(b *strings.Builder, scenario Scenario, stat ScenarioStats) {
	b.WriteString("| ")
	b.WriteString(escapeCell(scenario.Name))
	b.WriteString(" | ")
	b.WriteString(escapeCell(stat.Framework))
	b.WriteString(" | ")
	b.WriteString(escapeCell(stat.Protocol))
	b.WriteString(" | ")
	b.WriteString(escapeCell(stat.NegotiatedProtocol))
	b.WriteString(" | ")
	b.WriteString(escapeCell(stat.Backend))
	b.WriteString(" | ")
	b.WriteString(escapeCell(loopSummary(stat)))
	b.WriteString(" | ")
	b.WriteString(strconv.FormatInt(stat.TotalRequests, 10))
	b.WriteString(" | ")
	b.WriteString(strconv.FormatInt(stat.Errors, 10))
	b.WriteString(" | ")
	b.WriteString(formatFloat(stat.ThroughputOpsPerSec))
	b.WriteString(" | ")
	b.WriteString(strconv.FormatInt(stat.P99LatencyNanos, 10))
	b.WriteString(" | ")
	b.WriteString(strconv.FormatInt(stat.RSSBytes, 10))
	b.WriteString(" | ")
	b.WriteString(strconv.FormatInt(stat.GCCount, 10))
	b.WriteString(" | ")
	b.WriteString(stat.Elapsed.String())
	b.WriteString(" |\n")
}

func loopSummary(stat ScenarioStats) string {
	switch {
	case stat.Boss > 0 || stat.Workers > 0:
		parts := make([]string, 0, 3)
		if stat.Boss > 0 {
			parts = append(parts, "boss="+strconv.FormatInt(stat.Boss, 10))
		}
		if stat.Workers > 0 {
			parts = append(parts, "workers="+strconv.FormatInt(stat.Workers, 10))
		}
		if stat.ReadBufferBytes > 0 {
			parts = append(parts, "readBuffer="+strconv.FormatInt(stat.ReadBufferBytes, 10))
		}
		if stat.ReusePort {
			parts = append(parts, "reuseport=true")
		}
		if stat.Mmap {
			parts = append(parts, "mmap=true")
		}
		if stat.MmapBlockSize > 0 {
			parts = append(parts, "mmapBlock="+strconv.FormatInt(stat.MmapBlockSize, 10))
		}
		if stat.MmapBlocks > 0 {
			parts = append(parts, "mmapBlocks="+strconv.FormatInt(stat.MmapBlocks, 10))
		}
		if stat.IOUringFixedBuffers {
			parts = append(parts, "fixedBuffers=true")
		}
		if stat.IOUringMultishotAccept {
			parts = append(parts, "multishotAccept=true")
		}
		if stat.IOUringSQPoll {
			parts = append(parts, "sqpoll=true")
		}
		if stat.LatencySampleRate > 0 {
			parts = append(parts, "latencySampleRate="+strconv.FormatInt(stat.LatencySampleRate, 10))
		}
		return strings.Join(parts, " ")
	case stat.EventLoops > 0:
		return "eventLoops=" + strconv.FormatInt(stat.EventLoops, 10)
	default:
		return "-"
	}
}

func writeMetricRow(b *strings.Builder, scenario Scenario, metric BenchmarkMetric) {
	b.WriteString("| ")
	b.WriteString(escapeCell(scenario.Name))
	b.WriteString(" | ")
	b.WriteString(escapeCell(scenario.Framework))
	b.WriteString(" | ")
	b.WriteString(escapeCell(scenario.Protocol))
	b.WriteString(" | ")
	b.WriteString(escapeCell(metric.Name))
	b.WriteString(" | ")
	b.WriteString(formatFloat(metric.NsPerOp))
	b.WriteString(" | ")
	b.WriteString(strconv.FormatInt(metric.BytesPerOp, 10))
	b.WriteString(" | ")
	b.WriteString(strconv.FormatInt(metric.AllocsPerOp, 10))
	b.WriteString(" |\n")
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

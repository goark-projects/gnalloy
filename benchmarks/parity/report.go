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
	b.WriteString("\n## Scenarios\n\n")
	for _, result := range report.Scenarios {
		writeScenario(&b, result)
	}
	_, err := io.WriteString(w, b.String())
	return err
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

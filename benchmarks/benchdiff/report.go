package benchdiff

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"goark.dev/gnalloy/benchmarks/parity"
)

// Report 是一次上一版本对比的完整结果。
type Report struct {
	BaseLabel      string
	CandidateLabel string
	Command        []string
	Machine        parity.Machine
	Comparisons    []Comparison
	Missing        []Missing
}

// NewReport 从两段 go benchmark 输出生成对比报告。
func NewReport(baseLabel string, candidateLabel string, command []string, baseOutput string, candidateOutput string) Report {
	comparisons, missing := CompareSamples(ParseGoBenchOutput(baseOutput), ParseGoBenchOutput(candidateOutput))
	return Report{
		BaseLabel:      baseLabel,
		CandidateLabel: candidateLabel,
		Command:        append([]string(nil), command...),
		Machine:        parity.DetectMachine(),
		Comparisons:    comparisons,
		Missing:        missing,
	}
}

// WriteMarkdown 写出面向人工审阅的 Markdown 对比报告。
func WriteMarkdown(w io.Writer, report Report) error {
	if w == nil {
		return fmt.Errorf("%w: nil writer", ErrInvalidReport)
	}
	var b strings.Builder
	b.WriteString("# gnalloy benchmark diff\n\n")
	writeField(&b, "base", report.BaseLabel)
	writeField(&b, "candidate", report.CandidateLabel)
	if len(report.Command) > 0 {
		writeField(&b, "command", strings.Join(report.Command, " "))
	}
	if report.Machine.OS != "" {
		writeField(&b, "machine", report.Machine.OS+"/"+report.Machine.Arch)
		writeField(&b, "go", report.Machine.Go)
		writeField(&b, "cpus", strconv.Itoa(report.Machine.CPUs))
	}
	b.WriteString("\n")
	if len(report.Comparisons) > 0 {
		b.WriteString("| Package | Benchmark | Samples | Base ns/op | Candidate ns/op | ns/op delta | Base B/op | Candidate B/op | B/op delta | Base allocs/op | Candidate allocs/op | allocs/op delta |\n")
		b.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for _, comparison := range report.Comparisons {
			writeComparison(&b, comparison)
		}
		b.WriteString("\n")
	}
	if len(report.Missing) > 0 {
		b.WriteString("## Missing benchmarks\n\n")
		b.WriteString("| Package | Benchmark | Missing side |\n")
		b.WriteString("| --- | --- | --- |\n")
		for _, missing := range report.Missing {
			b.WriteString("| ")
			b.WriteString(escapeCell(missing.Package))
			b.WriteString(" | ")
			b.WriteString(escapeCell(missing.Benchmark))
			b.WriteString(" | ")
			b.WriteString(escapeCell(missing.Side))
			b.WriteString(" |\n")
		}
		b.WriteString("\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeField(b *strings.Builder, key string, value string) {
	if strings.TrimSpace(value) == "" {
		value = "-"
	}
	b.WriteString("**")
	b.WriteString(key)
	b.WriteString(":** ")
	b.WriteString(value)
	b.WriteString("\n")
}

func writeComparison(b *strings.Builder, comparison Comparison) {
	b.WriteString("| ")
	b.WriteString(escapeCell(comparison.Package))
	b.WriteString(" | ")
	b.WriteString(escapeCell(comparison.Benchmark))
	b.WriteString(" | ")
	b.WriteString(strconv.Itoa(comparison.Candidate.Samples))
	b.WriteString("/")
	b.WriteString(strconv.Itoa(comparison.Base.Samples))
	b.WriteString(" | ")
	b.WriteString(formatNumber(comparison.Base.NsPerOp))
	b.WriteString(" | ")
	b.WriteString(formatNumber(comparison.Candidate.NsPerOp))
	b.WriteString(" | ")
	b.WriteString(formatPercent(comparison.Base.NsPerOp, comparison.Candidate.NsPerOp, comparison.NsPerOpChangePercent))
	b.WriteString(" | ")
	b.WriteString(formatNumber(comparison.Base.BytesPerOp))
	b.WriteString(" | ")
	b.WriteString(formatNumber(comparison.Candidate.BytesPerOp))
	b.WriteString(" | ")
	b.WriteString(formatPercent(comparison.Base.BytesPerOp, comparison.Candidate.BytesPerOp, comparison.BytesPerOpChangePercent))
	b.WriteString(" | ")
	b.WriteString(formatNumber(comparison.Base.AllocsPerOp))
	b.WriteString(" | ")
	b.WriteString(formatNumber(comparison.Candidate.AllocsPerOp))
	b.WriteString(" | ")
	b.WriteString(formatPercent(comparison.Base.AllocsPerOp, comparison.Candidate.AllocsPerOp, comparison.AllocsPerOpChangePercent))
	b.WriteString(" |\n")
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func formatPercent(base float64, candidate float64, value float64) string {
	if base == 0 {
		if candidate > 0 {
			return "+inf"
		}
		return "0.00%"
	}
	if value > 0 {
		return "+" + formatNumber(value) + "%"
	}
	return formatNumber(value) + "%"
}

func escapeCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "|", "\\|")
}

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"goark.dev/gnalloy/benchmarks/benchdiff"
)

func main() {
	baseRef := flag.String("base", "HEAD", "git ref used as baseline")
	packages := flag.String("packages", "./buffer", "comma-separated package list")
	bench := flag.String("bench", ".", "go benchmark regexp")
	count := flag.Int("count", 5, "sample count for each version")
	benchtime := flag.String("benchtime", "", "optional go test -benchtime value")
	timeout := flag.Duration("timeout", 10*time.Minute, "overall comparison timeout")
	outPath := flag.String("out", "", "markdown output path, empty writes stdout")
	goCommand := flag.String("go", "go", "go command")
	gitCommand := flag.String("git", "git", "git command")
	flag.Parse()

	report, err := benchdiff.Runner{
		BaseRef:   *baseRef,
		Go:        *goCommand,
		Git:       *gitCommand,
		Bench:     *bench,
		Benchtime: *benchtime,
		Packages:  splitCSV(*packages),
		Count:     *count,
		Timeout:   *timeout,
	}.Run(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writeReport(*outPath, report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeReport(outPath string, report benchdiff.Report) error {
	if strings.TrimSpace(outPath) == "" {
		return benchdiff.WriteMarkdown(os.Stdout, report)
	}
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	return benchdiff.WriteMarkdown(out, report)
}

func splitCSV(value string) []string {
	fields := strings.Split(value, ",")
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

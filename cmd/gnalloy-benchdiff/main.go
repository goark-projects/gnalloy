package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"goark.dev/gnalloy/benchmarks/benchdiff"
	"goark.dev/gnalloy/benchmarks/microbench"
)

func main() {
	baseRef := flag.String("base", "HEAD", "git ref used as baseline")
	suite := flag.String("suite", "", "microbenchmark suite name")
	listSuites := flag.Bool("list-suites", false, "list available microbenchmark suites")
	packages := flag.String("packages", "", "comma-separated package list")
	bench := flag.String("bench", "", "go benchmark regexp")
	count := flag.Int("count", 5, "sample count for each version")
	benchtime := flag.String("benchtime", "", "optional go test -benchtime value")
	timeout := flag.Duration("timeout", 10*time.Minute, "overall comparison timeout")
	outPath := flag.String("out", "", "markdown output path, empty writes stdout")
	goCommand := flag.String("go", "go", "go command")
	gitCommand := flag.String("git", "git", "git command")
	flag.Parse()

	if *listSuites {
		writeSuites(os.Stdout)
		return
	}
	selectedPackages, selectedBench, err := resolveBenchmarkSelection(*suite, *packages, *bench)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	report, err := benchdiff.Runner{
		BaseRef:   *baseRef,
		Go:        *goCommand,
		Git:       *gitCommand,
		Bench:     selectedBench,
		Benchtime: *benchtime,
		Packages:  selectedPackages,
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

func resolveBenchmarkSelection(suiteName string, packageCSV string, bench string) ([]string, string, error) {
	packages := splitCSV(packageCSV)
	bench = strings.TrimSpace(bench)
	if strings.TrimSpace(suiteName) != "" {
		suite, ok := microbench.Lookup(suiteName)
		if !ok {
			return nil, "", fmt.Errorf("unknown microbenchmark suite %q", suiteName)
		}
		if len(packages) == 0 {
			packages = suite.Packages()
		}
		if bench == "" {
			bench = suite.BenchmarkRegexp()
		}
	}
	if len(packages) == 0 {
		packages = []string{"./buffer"}
	}
	if bench == "" {
		bench = "."
	}
	return packages, bench, nil
}

func writeSuites(w io.Writer) {
	for _, suite := range microbench.Suites() {
		fmt.Fprintf(w, "%s\t%s\n", suite.Name, suite.Description)
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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"goark.dev/gnalloy/benchmarks/parity"
)

func main() {
	configPath := flag.String("config", "benchmarks/parity/baseline.json", "parity benchmark json spec")
	outPath := flag.String("out", "", "report output path, empty writes stdout")
	format := flag.String("format", "markdown", "report format: markdown or json")
	dryRun := flag.Bool("dry-run", false, "validate spec and render report without executing commands")
	strictExternal := flag.Bool("strict-external", false, "fail unless external Netty/gnet/netpoll harness commands are enabled and available")
	timeout := flag.Duration("timeout", 0, "overall timeout")
	flag.Parse()

	if err := run(*configPath, *outPath, parity.Format(*format), *dryRun, *strictExternal, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(configPath string, outPath string, format parity.Format, dryRun bool, strictExternal bool, timeout time.Duration) error {
	file, err := os.Open(configPath)
	if err != nil {
		return err
	}
	defer file.Close()
	spec, err := parity.LoadSpec(file)
	if err != nil {
		return err
	}
	if strictExternal {
		if err := parity.ValidateExternalHarnesses(spec, parity.ExternalHarnessOptions{}); err != nil {
			return err
		}
	}
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	report, err := parity.Runner{DryRun: dryRun}.Run(ctx, spec)
	if err != nil {
		return err
	}
	if outPath == "" {
		return parity.WriteReport(os.Stdout, report, format)
	}
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	return parity.WriteReport(out, report, format)
}

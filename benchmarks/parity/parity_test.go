package parity

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadSpecRejectsInvalidScenario(t *testing.T) {
	_, err := LoadSpec(strings.NewReader(`{"scenarios":[{"name":"netty"}]}`))
	if !errors.Is(err, ErrInvalidScenario) {
		t.Fatalf("err=%v, want ErrInvalidScenario", err)
	}
}

func TestLoadSpecAllowsSkippedExternalScenarioWithoutCommand(t *testing.T) {
	spec, err := LoadSpec(strings.NewReader(`{
		"scenarios": [{
			"name": "netty tcp echo",
			"framework": "netty",
			"protocol": "tcp-echo",
			"skip": true,
			"skipReason": "external harness is not installed"
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !spec.Scenarios[0].Skip || spec.Scenarios[0].SkipReason == "" {
		t.Fatalf("scenario=%+v", spec.Scenarios[0])
	}
}

func TestBaselineSpecLoads(t *testing.T) {
	file, err := os.Open("baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) < 5 {
		t.Fatalf("scenarios=%d, want external baseline set", len(spec.Scenarios))
	}
	hasSkippedExternal := false
	for _, scenario := range spec.Scenarios {
		if scenario.Skip && scenario.SkipReason != "" && scenario.Framework != "gnalloy" {
			hasSkippedExternal = true
		}
	}
	if !hasSkippedExternal {
		t.Fatal("baseline must include skipped external framework scenarios")
	}
}

func TestValidateExternalHarnessesRejectsSkippedScenario(t *testing.T) {
	spec := Spec{
		Name: "strict",
		Scenarios: []Scenario{{
			Name:       "netty tcp echo",
			Framework:  "netty",
			Protocol:   "tcp-echo",
			Skip:       true,
			SkipReason: "external harness missing",
		}},
	}
	err := ValidateExternalHarnesses(spec, ExternalHarnessOptions{})
	if !errors.Is(err, ErrExternalHarness) {
		t.Fatalf("err=%v, want ErrExternalHarness", err)
	}
}

func TestInspectExternalHarnessesChecksExpandedCommand(t *testing.T) {
	spec := Spec{
		Name:      "strict",
		Variables: map[string]string{"GNET_BENCH": "./external/gnet-bench"},
		Scenarios: []Scenario{{
			Name:      "gnet tcp echo",
			Framework: "gnet",
			Protocol:  "tcp-echo",
			Command:   []string{"${GNET_BENCH}"},
			Tags:      []string{"external"},
		}},
	}
	report, err := InspectExternalHarnesses(spec, ExternalHarnessOptions{
		Stat: func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.ExternalScenarios != 1 || report.Missing != 1 || len(report.Issues) != 1 {
		t.Fatalf("report=%+v", report)
	}
	if !strings.Contains(report.Issues[0].Reason, "./external/gnet-bench") {
		t.Fatalf("issue=%+v", report.Issues[0])
	}
}

func TestValidateExternalHarnessesAcceptsReadyCommand(t *testing.T) {
	spec := Spec{
		Name: "strict",
		Scenarios: []Scenario{{
			Name:      "netpoll tcp echo",
			Framework: "netpoll",
			Protocol:  "tcp-echo",
			Command:   []string{"netpoll-bench"},
		}},
	}
	err := ValidateExternalHarnesses(spec, ExternalHarnessOptions{
		LookPath: func(file string) (string, error) {
			if file != "netpoll-bench" {
				t.Fatalf("file=%q", file)
			}
			return "/usr/local/bin/netpoll-bench", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunnerDryRunProducesSkippedResults(t *testing.T) {
	spec := Spec{
		Name: "dry-run",
		Scenarios: []Scenario{{
			Name:      "gnalloy-echo",
			Framework: "gnalloy",
			Protocol:  "tcp-echo",
			Command:   []string{"gnalloy-bench"},
		}},
	}
	report, err := Runner{DryRun: true, Now: fixedNow()}.Run(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Scenarios) != 1 || !report.Scenarios[0].Skipped {
		t.Fatalf("report=%+v", report)
	}
}

func TestRunnerSkipsScenarioMarkedSkip(t *testing.T) {
	spec := Spec{
		Name: "skip",
		Scenarios: []Scenario{{
			Name:       "netty",
			Framework:  "netty",
			Protocol:   "tcp-echo",
			Skip:       true,
			SkipReason: "external harness missing",
		}},
	}
	report, err := Runner{Now: fixedNow()}.Run(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	result := report.Scenarios[0]
	if !result.Skipped || result.Output != "external harness missing" || result.ExitCode != 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestRunnerCapturesCommandOutput(t *testing.T) {
	if os.Getenv("GNALLOY_PARITY_HELPER") == "1" {
		os.Stdout.WriteString("helper-output\nBenchmarkEcho-16 1000 123 ns/op 64 B/op 2 allocs/op\n")
		return
	}
	spec := Spec{
		Name: "helper",
		Scenarios: []Scenario{{
			Name:      "helper",
			Framework: "test",
			Protocol:  "raw",
			Command:   []string{os.Args[0], "-test.run=TestRunnerCapturesCommandOutput"},
			Env:       map[string]string{"GNALLOY_PARITY_HELPER": "1"},
			Timeout:   Duration(time.Second),
		}},
	}
	report, err := Runner{Now: fixedNow()}.Run(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	result := report.Scenarios[0]
	if result.ExitCode != 0 || !strings.Contains(result.Output, "helper-output") {
		t.Fatalf("result=%+v", result)
	}
	if len(result.Metrics) != 1 || result.Metrics[0].Name != "BenchmarkEcho-16" || result.Metrics[0].NsPerOp != 123 {
		t.Fatalf("metrics=%+v", result.Metrics)
	}
}

func TestRunnerExpandsScenarioVariables(t *testing.T) {
	if os.Getenv("GNALLOY_PARITY_HELPER") == "2" {
		os.Stdout.WriteString(os.Getenv("GNALLOY_PARITY_PAYLOAD"))
		return
	}
	spec := Spec{
		Name:      "vars",
		Variables: map[string]string{"PAYLOAD": "1KiB"},
		Scenarios: []Scenario{{
			Name:      "helper",
			Framework: "test",
			Protocol:  "raw",
			Payload:   "${PAYLOAD}",
			Command:   []string{os.Args[0], "-test.run=TestRunnerExpandsScenarioVariables"},
			Env: map[string]string{
				"GNALLOY_PARITY_HELPER":  "2",
				"GNALLOY_PARITY_PAYLOAD": "${PAYLOAD}",
			},
			Timeout: Duration(time.Second),
		}},
	}
	report, err := Runner{Now: fixedNow()}.Run(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	result := report.Scenarios[0]
	if result.Scenario.Payload != "1KiB" || !strings.Contains(result.Output, "1KiB") {
		t.Fatalf("result=%+v", result)
	}
}

func TestWriteMarkdownReportIncludesMachineAndScenario(t *testing.T) {
	report := Report{
		Name:    "parity",
		Machine: Machine{Hostname: "host", OS: "linux", Arch: "amd64", CPUs: 8, Go: "go1.25"},
		Scenarios: []ScenarioResult{{
			Scenario: Scenario{Name: "netty", Framework: "netty", Protocol: "tcp", Command: []string{"java", "-jar", "bench.jar"}},
			Output:   "ok\n",
			Metrics:  []BenchmarkMetric{{Name: "BenchmarkEcho-16", Iterations: 1000, NsPerOp: 123, BytesPerOp: 64, AllocsPerOp: 2}},
		}},
	}
	var out bytes.Buffer
	if err := WriteReport(&out, report, FormatMarkdown); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"# parity", "| os | linux |", "## Summary", "BenchmarkEcho-16", "### netty", "java -jar bench.jar", "ok"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
}

func TestWriteJSONReport(t *testing.T) {
	report := Report{Name: "json", Scenarios: []ScenarioResult{{Scenario: Scenario{Name: "s", Framework: "f", Protocol: "p", Command: []string{"cmd"}}}}}
	var out bytes.Buffer
	if err := WriteReport(&out, report, FormatJSON); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name": "json"`) {
		t.Fatalf("json=%s", out.String())
	}
}

func fixedNow() func() time.Time {
	now := time.Unix(100, 0).UTC()
	return func() time.Time { return now }
}

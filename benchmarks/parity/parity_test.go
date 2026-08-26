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

func TestRunnerCapturesCommandOutput(t *testing.T) {
	if os.Getenv("GNALLOY_PARITY_HELPER") == "1" {
		os.Stdout.WriteString("helper-output\n")
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
}

func TestWriteMarkdownReportIncludesMachineAndScenario(t *testing.T) {
	report := Report{
		Name:    "parity",
		Machine: Machine{Hostname: "host", OS: "linux", Arch: "amd64", CPUs: 8, Go: "go1.25"},
		Scenarios: []ScenarioResult{{
			Scenario: Scenario{Name: "netty", Framework: "netty", Protocol: "tcp", Command: []string{"java", "-jar", "bench.jar"}},
			Output:   "ok\n",
		}},
	}
	var out bytes.Buffer
	if err := WriteReport(&out, report, FormatMarkdown); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"# parity", "| os | linux |", "### netty", "java -jar bench.jar", "ok"} {
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

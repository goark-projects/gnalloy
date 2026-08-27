package parity

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestLoadSpecRejectsNegativeSampling(t *testing.T) {
	_, err := LoadSpec(strings.NewReader(`{
		"scenarios": [{
			"name": "netty",
			"framework": "netty",
			"protocol": "tcp-echo",
			"warmup": -1,
			"command": ["netty-bench"]
		}]
	}`))
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
	enabledExternal := 0
	for _, scenario := range spec.Scenarios {
		if isExternalScenario(scenario, externalFrameworkSet(nil)) {
			if scenario.Skip {
				t.Fatalf("external scenario %q must be enabled in baseline", scenario.Name)
			}
			if len(scenario.Command) == 0 {
				t.Fatalf("external scenario %q has empty command", scenario.Name)
			}
			enabledExternal++
		}
	}
	if enabledExternal != 3 {
		t.Fatalf("enabled external scenarios=%d, want 3", enabledExternal)
	}
}

func TestTCPMatrixSpecLoads(t *testing.T) {
	file, err := os.Open("tcp-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) < 9 {
		t.Fatalf("scenarios=%d, want tcp matrix scenarios", len(spec.Scenarios))
	}
	for _, scenario := range spec.Scenarios {
		if scenario.Warmup != 1 || scenario.Repeat != 3 {
			t.Fatalf("scenario %q warmup=%d repeat=%d, want 1/3", scenario.Name, scenario.Warmup, scenario.Repeat)
		}
	}
}

func TestBaselineNettyTCPEchoUsesNativeEpoll(t *testing.T) {
	file, err := os.Open("baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	var scenario Scenario
	found := false
	for _, candidate := range spec.Scenarios {
		if candidate.Framework == "netty" && candidate.Protocol == "tcp-echo" {
			scenario = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatal("netty tcp echo scenario missing")
	}
	command := strings.Join(scenario.Command, " ")
	for _, want := range []string{"--backend epoll", "--connections ${CONNECTIONS}", "--messages ${MESSAGES}"} {
		if !strings.Contains(command, want) {
			t.Fatalf("missing %q in command %q", want, command)
		}
	}
	if scenario.Backend != "epoll" {
		t.Fatalf("backend=%q, want epoll", scenario.Backend)
	}
}

func TestBaselineGnalloyTCPEchoUsesSameLoadModel(t *testing.T) {
	file, err := os.Open("baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	var scenario Scenario
	found := false
	for _, candidate := range spec.Scenarios {
		if candidate.Name == "gnalloy tcp echo" {
			scenario = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatal("gnalloy tcp echo scenario missing")
	}
	command := strings.Join(scenario.Command, " ")
	for _, want := range []string{"${GNALLOY_BENCH}", "-connections ${CONNECTIONS}", "-messages ${MESSAGES}"} {
		if !strings.Contains(command, want) {
			t.Fatalf("missing %q in command %q", want, command)
		}
	}
	if strings.Contains(command, "go test") || strings.Contains(command, "BenchmarkNativeTCPEchoRoundTrip") {
		t.Fatalf("gnalloy tcp echo must not use single-connection go benchmark: %q", command)
	}
	if !hasScenarioTag(scenario, "parity-harness") {
		t.Fatalf("gnalloy tcp echo tags=%v, want parity-harness", scenario.Tags)
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

func TestValidateExternalHarnessesChecksJavaJarArgument(t *testing.T) {
	spec := Spec{
		Name: "strict",
		Scenarios: []Scenario{{
			Name:      "netty tcp echo",
			Framework: "netty",
			Protocol:  "tcp-echo",
			Command:   []string{"java", "-jar", "./benchmarks/external/bin/netty-bench.jar"},
		}},
	}
	err := ValidateExternalHarnesses(spec, ExternalHarnessOptions{
		LookPath: func(file string) (string, error) {
			if file != "java" {
				t.Fatalf("file=%q", file)
			}
			return "/usr/bin/java", nil
		},
		Stat: func(string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	})
	if !errors.Is(err, ErrExternalHarness) {
		t.Fatalf("err=%v, want ErrExternalHarness", err)
	}
}

func TestBaselineExternalHarnessesCanPassStrictGateWithRepoArtifacts(t *testing.T) {
	file, err := os.Open("baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	err = ValidateExternalHarnesses(spec, ExternalHarnessOptions{
		LookPath: func(file string) (string, error) {
			if file == "java" {
				return "/usr/bin/java", nil
			}
			return "", os.ErrNotExist
		},
		Stat: func(name string) (os.FileInfo, error) {
			switch filepath.ToSlash(filepath.Clean(name)) {
			case "benchmarks/external/bin/netty-bench.jar",
				"benchmarks/external/bin/gnalloy-bench",
				"benchmarks/external/bin/gnalloy-bench.exe",
				"benchmarks/external/bin/gnet-bench",
				"benchmarks/external/bin/gnet-bench.exe",
				"benchmarks/external/bin/netpoll-bench",
				"benchmarks/external/bin/netpoll-bench.exe":
				return fakeFileInfo{name: filepath.Base(name)}, nil
			default:
				return nil, os.ErrNotExist
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateExternalHarnessesChecksGnalloyParityHarness(t *testing.T) {
	spec := Spec{
		Name:      "strict",
		Variables: map[string]string{"GNALLOY_BENCH": "./benchmarks/external/bin/gnalloy-bench"},
		Scenarios: []Scenario{{
			Name:      "gnalloy tcp echo",
			Framework: "gnalloy",
			Protocol:  "tcp-echo",
			Command:   []string{"${GNALLOY_BENCH}", "-protocol", "tcp-echo"},
			Tags:      []string{"local", "parity-harness"},
		}},
	}
	err := ValidateExternalHarnesses(spec, ExternalHarnessOptions{
		Stat: func(string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	})
	if !errors.Is(err, ErrExternalHarness) {
		t.Fatalf("err=%v, want ErrExternalHarness", err)
	}
}

func TestPathCommandCandidatesAddsWindowsExecutableSuffix(t *testing.T) {
	candidates := pathCommandCandidates("./benchmarks/external/bin/gnet-bench", ".", "windows")
	joined := strings.Join(candidates, "|")
	want := filepath.Join("benchmarks", "external", "bin", "gnet-bench.exe")
	if !strings.Contains(joined, want) && !strings.Contains(joined, "benchmarks/external/bin/gnet-bench.exe") {
		t.Fatalf("candidates=%v", candidates)
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
		os.Stdout.WriteString("helper-output\nframework=gnalloy protocol=tcp-echo backend=iocp payload=1024 connections=2 messages=3 total=6 errors=0 elapsed=2ms throughput=3000.50 ops/s\nBenchmarkEcho-16 1000 123 ns/op 64 B/op 2 allocs/op\n")
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
			Timeout:   Duration(5 * time.Second),
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
	if len(result.Stats) != 1 || result.Stats[0].TotalRequests != 6 || result.Stats[0].ThroughputOpsPerSec != 3000.50 {
		t.Fatalf("stats=%+v", result.Stats)
	}
}

func TestRunnerRepeatsScenarioAndDropsWarmupOutput(t *testing.T) {
	if os.Getenv("GNALLOY_PARITY_HELPER") == "repeat" {
		os.Stdout.WriteString("framework=netty protocol=tcp-echo backend=epoll payload=16 connections=1 messages=2 total=2 errors=0 elapsed=PT0.000004S throughput=500000.00 ops/s\nBenchmarkNettyTCPEcho-8 2 2000 ns/op\n")
		return
	}
	spec := Spec{
		Name: "repeat",
		Scenarios: []Scenario{{
			Name:      "helper",
			Framework: "netty",
			Protocol:  "tcp-echo",
			Warmup:    1,
			Repeat:    2,
			Command:   []string{os.Args[0], "-test.run=TestRunnerRepeatsScenarioAndDropsWarmupOutput"},
			Env:       map[string]string{"GNALLOY_PARITY_HELPER": "repeat"},
			Timeout:   Duration(5 * time.Second),
		}},
	}
	report, err := Runner{Now: fixedNow()}.Run(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	result := report.Scenarios[0]
	if len(result.Samples) != 2 || len(result.Stats) != 2 || len(result.Metrics) != 2 {
		t.Fatalf("result=%+v", result)
	}
	if strings.Count(result.Output, "framework=netty") != 2 {
		t.Fatalf("warmup output leaked or repeat output missing: %q", result.Output)
	}
	if result.Stats[0].Elapsed != 4*time.Microsecond {
		t.Fatalf("elapsed=%v, want 4us", result.Stats[0].Elapsed)
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
			Timeout: Duration(5 * time.Second),
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
			Scenario: Scenario{Name: "netty", Framework: "netty", Protocol: "tcp", Repeat: 2, Command: []string{"java", "-jar", "bench.jar"}},
			Output:   "ok\n",
			Stats: []ScenarioStats{
				{Framework: "netty", Protocol: "tcp", Backend: "epoll", TotalRequests: 100, Errors: 0, Elapsed: 100 * time.Millisecond, ThroughputOpsPerSec: 1000},
				{Framework: "netty", Protocol: "tcp", Backend: "epoll", TotalRequests: 100, Errors: 0, Elapsed: 80 * time.Millisecond, ThroughputOpsPerSec: 1250},
			},
			Metrics: []BenchmarkMetric{{Name: "BenchmarkEcho-16", Iterations: 1000, NsPerOp: 123, BytesPerOp: 64, AllocsPerOp: 2}},
		}},
	}
	var out bytes.Buffer
	if err := WriteReport(&out, report, FormatMarkdown); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"# parity", "| os | linux |", "## Summary", "Throughput median", "1125", "BenchmarkEcho-16", "### netty", "java -jar bench.jar", "ok"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
}

func TestParseScenarioStats(t *testing.T) {
	stats := ParseScenarioStats("framework=gnalloy protocol=tcp-echo backend=iocp payload=1024 connections=256 messages=100000 total=25600000 errors=0 elapsed=3.2s throughput=8000000.25 ops/s\n")
	if len(stats) != 1 {
		t.Fatalf("stats=%+v, want one", stats)
	}
	stat := stats[0]
	if stat.Framework != "gnalloy" || stat.Protocol != "tcp-echo" || stat.Backend != "iocp" {
		t.Fatalf("stat=%+v", stat)
	}
	if stat.PayloadBytes != 1024 || stat.Connections != 256 || stat.Messages != 100000 || stat.TotalRequests != 25600000 || stat.Errors != 0 {
		t.Fatalf("stat=%+v", stat)
	}
	if stat.Elapsed != 3200*time.Millisecond || stat.ThroughputOpsPerSec != 8000000.25 {
		t.Fatalf("stat=%+v", stat)
	}
}

func TestParseScenarioStatsParsesJavaDuration(t *testing.T) {
	stats := ParseScenarioStats("framework=netty protocol=tcp-echo backend=epoll payload=1024 connections=256 messages=100000 total=25600000 errors=0 elapsed=PT2M22.38974812S throughput=179788.22 ops/s\n")
	if len(stats) != 1 {
		t.Fatalf("stats=%+v, want one", stats)
	}
	if stats[0].Elapsed != 2*time.Minute+22389748120*time.Nanosecond {
		t.Fatalf("elapsed=%v", stats[0].Elapsed)
	}
}

func TestWriteJSONReport(t *testing.T) {
	report := Report{Name: "json", Scenarios: []ScenarioResult{{
		Scenario: Scenario{Name: "s", Framework: "f", Protocol: "p", Command: []string{"cmd"}},
		Stats:    []ScenarioStats{{Framework: "f", Protocol: "p", TotalRequests: 1, Errors: 0}},
	}}}
	var out bytes.Buffer
	if err := WriteReport(&out, report, FormatJSON); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name": "json"`) {
		t.Fatalf("json=%s", out.String())
	}
	if !strings.Contains(out.String(), `"errors": 0`) {
		t.Fatalf("json=%s", out.String())
	}
}

func fixedNow() func() time.Time {
	now := time.Unix(100, 0).UTC()
	return func() time.Time { return now }
}

type fakeFileInfo struct {
	name string
}

func (f fakeFileInfo) Name() string     { return f.name }
func (fakeFileInfo) Size() int64        { return 1 }
func (fakeFileInfo) Mode() os.FileMode  { return 0o755 }
func (fakeFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (fakeFileInfo) IsDir() bool        { return false }
func (fakeFileInfo) Sys() any           { return nil }

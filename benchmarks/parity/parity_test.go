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

func TestTCPMatrixIncludesOptimizedIOUringScenario(t *testing.T) {
	file, err := os.Open("tcp-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range spec.Scenarios {
		if scenario.Name != "gnalloy iouring fixed tcp echo 1KiB" {
			continue
		}
		command := strings.Join(scenario.Command, " ")
		for _, want := range []string{"-backend iouring", "-mmap", "-mmap-blocks 512", "-iouring-fixed-buffers", "-iouring-multishot-accept"} {
			if !strings.Contains(command, want) {
				t.Fatalf("missing %q in command %q", want, command)
			}
		}
		return
	}
	t.Fatal("optimized iouring scenario missing")
}

func TestWindowsTCPMatrixSpecLoads(t *testing.T) {
	file, err := os.Open("windows-tcp.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) != 7 {
		t.Fatalf("scenarios=%d, want windows tcp matrix scenarios", len(spec.Scenarios))
	}
	for _, scenario := range spec.Scenarios {
		if scenario.Warmup != 1 || scenario.Repeat != 3 {
			t.Fatalf("scenario %q warmup=%d repeat=%d, want 1/3", scenario.Name, scenario.Warmup, scenario.Repeat)
		}
		if scenario.Framework == "netpoll" {
			t.Fatalf("windows matrix must not include netpoll scenario: %+v", scenario)
		}
		if scenario.Framework == "netty" && scenario.Backend != "nio" {
			t.Fatalf("windows netty backend=%q, want nio", scenario.Backend)
		}
		if scenario.Framework == "gnalloy" && scenario.Backend != "iocp" {
			t.Fatalf("windows gnalloy backend=%q, want iocp", scenario.Backend)
		}
	}
}

func TestHTTPS1ALPNMatrixSpecLoads(t *testing.T) {
	file, err := os.Open("https1-alpn-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) != 4 {
		t.Fatalf("scenarios=%d, want https1 alpn matrix scenarios", len(spec.Scenarios))
	}
	for _, scenario := range spec.Scenarios {
		command := strings.Join(scenario.Command, " ")
		if scenario.Protocol != "https1" || !strings.Contains(command, "alpn") {
			t.Fatalf("scenario=%+v command=%q", scenario, command)
		}
		if scenario.Framework != "gnalloy" && scenario.Framework != "netty" {
			t.Fatalf("framework=%q, want gnalloy or netty", scenario.Framework)
		}
	}
}

func TestLinuxHTTPS1ALPNMatrixSpecLoads(t *testing.T) {
	file, err := os.Open("linux-https1-alpn-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) != 4 {
		t.Fatalf("scenarios=%d, want linux https1 alpn matrix scenarios", len(spec.Scenarios))
	}
	for _, scenario := range spec.Scenarios {
		command := strings.Join(scenario.Command, " ")
		if scenario.Protocol != "https1" || scenario.Backend != "epoll" {
			t.Fatalf("scenario=%+v", scenario)
		}
		if !strings.Contains(command, "epoll") || !strings.Contains(command, "alpn") {
			t.Fatalf("command=%q", command)
		}
	}
}

func TestHTTP2MatrixSpecLoads(t *testing.T) {
	file, err := os.Open("http2-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) != 8 {
		t.Fatalf("scenarios=%d, want http2 matrix scenarios", len(spec.Scenarios))
	}
	for _, scenario := range spec.Scenarios {
		command := strings.Join(scenario.Command, " ")
		if scenario.Protocol != "http2" && scenario.Protocol != "https2" {
			t.Fatalf("scenario=%+v", scenario)
		}
		if scenario.Protocol == "https2" && !strings.Contains(command, "-alpn ${ALPN}") {
			t.Fatalf("https2 command missing h2 ALPN: %q", command)
		}
		if scenario.Framework == "gnalloy" && scenario.Backend != "iocp" {
			t.Fatalf("gnalloy backend=%q, want iocp", scenario.Backend)
		}
		if scenario.Framework == "netty" && scenario.Backend != "nio" {
			t.Fatalf("netty backend=%q, want nio", scenario.Backend)
		}
		if scenario.Framework != "gnalloy" && scenario.Framework != "netty" {
			t.Fatalf("framework=%q, want gnalloy or netty", scenario.Framework)
		}
		if scenario.Warmup != 1 || scenario.Repeat != 3 {
			t.Fatalf("scenario %q warmup=%d repeat=%d, want 1/3", scenario.Name, scenario.Warmup, scenario.Repeat)
		}
	}
}

func TestTLSVersionMatrixSpecLoads(t *testing.T) {
	file, err := os.Open("tls-version-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) != 20 {
		t.Fatalf("scenarios=%d, want tls version matrix scenarios", len(spec.Scenarios))
	}
	seen := map[string]bool{}
	for _, scenario := range spec.Scenarios {
		command := strings.Join(scenario.Command, " ")
		if !strings.Contains(command, "tls-version") {
			t.Fatalf("scenario %q missing tls-version: %q", scenario.Name, command)
		}
		if scenario.Framework != "gnalloy" && scenario.Framework != "netty" {
			t.Fatalf("framework=%q, want gnalloy or netty", scenario.Framework)
		}
		if scenario.Framework == "gnalloy" && scenario.Backend != "iocp" {
			t.Fatalf("gnalloy backend=%q, want iocp", scenario.Backend)
		}
		if scenario.Protocol == "https2" && strings.Contains(command, "1.1") {
			t.Fatalf("https2 must not use TLS 1.1: %q", command)
		}
		assertTLSCipherSuiteCommand(t, command)
		for _, version := range []string{"1.1", "1.2", "1.3"} {
			if strings.Contains(command, "tls-version") && strings.Contains(command, version) {
				seen[version] = true
			}
		}
	}
	for _, version := range []string{"1.1", "1.2", "1.3"} {
		if !seen[version] {
			t.Fatalf("tls version %s missing", version)
		}
	}
}

func TestLinuxTLSVersionMatrixSpecLoads(t *testing.T) {
	file, err := os.Open("linux-tls-version-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) != 20 {
		t.Fatalf("scenarios=%d, want linux tls version matrix scenarios", len(spec.Scenarios))
	}
	for _, scenario := range spec.Scenarios {
		command := strings.Join(scenario.Command, " ")
		if !strings.Contains(command, "tls-version") {
			t.Fatalf("scenario %q missing tls-version: %q", scenario.Name, command)
		}
		if scenario.Backend != "epoll" {
			t.Fatalf("backend=%q, want epoll", scenario.Backend)
		}
		if scenario.Protocol == "https2" && strings.Contains(command, "1.1") {
			t.Fatalf("https2 must not use TLS 1.1: %q", command)
		}
		assertTLSCipherSuiteCommand(t, command)
	}
}

func assertTLSCipherSuiteCommand(t *testing.T, command string) {
	t.Helper()
	switch {
	case strings.Contains(command, "tls-version 1.1"), strings.Contains(command, "tls-version 1.2"):
		if !strings.Contains(command, "cipher-suites") {
			t.Fatalf("TLS 1.1/1.2 scenario missing cipher-suites: %q", command)
		}
	case strings.Contains(command, "tls-version 1.3"):
		if strings.Contains(command, "cipher-suites") {
			t.Fatalf("TLS 1.3 scenario must use runtime-default cipher suites: %q", command)
		}
	}
}

func TestLinuxHTTP2MatrixSpecLoads(t *testing.T) {
	file, err := os.Open("linux-http2-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) != 8 {
		t.Fatalf("scenarios=%d, want linux http2 matrix scenarios", len(spec.Scenarios))
	}
	for _, scenario := range spec.Scenarios {
		command := strings.Join(scenario.Command, " ")
		if scenario.Protocol != "http2" && scenario.Protocol != "https2" {
			t.Fatalf("scenario=%+v", scenario)
		}
		if scenario.Backend != "epoll" {
			t.Fatalf("backend=%q, want epoll", scenario.Backend)
		}
		if scenario.Protocol == "https2" && !strings.Contains(command, "-alpn ${ALPN}") {
			t.Fatalf("https2 command missing h2 ALPN: %q", command)
		}
		if scenario.Framework != "gnalloy" && scenario.Framework != "netty" {
			t.Fatalf("framework=%q, want gnalloy or netty", scenario.Framework)
		}
		if scenario.Warmup != 1 || scenario.Repeat != 3 {
			t.Fatalf("scenario %q warmup=%d repeat=%d, want 1/3", scenario.Name, scenario.Warmup, scenario.Repeat)
		}
	}
}

func TestHTTP3MatrixSpecLoads(t *testing.T) {
	file, err := os.Open("http3-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) != 4 {
		t.Fatalf("scenarios=%d, want http3 matrix scenarios", len(spec.Scenarios))
	}
	if spec.Variables["ALPN"] != "h3" || spec.Variables["TLS_VERSION"] != "1.3" {
		t.Fatalf("variables=%v, want h3/TLS1.3", spec.Variables)
	}
	for _, scenario := range spec.Scenarios {
		assertHTTP3MatrixScenario(t, scenario, "nio")
	}
}

func TestLinuxHTTP3MatrixSpecLoads(t *testing.T) {
	file, err := os.Open("linux-http3-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) != 4 {
		t.Fatalf("scenarios=%d, want linux http3 matrix scenarios", len(spec.Scenarios))
	}
	if spec.Variables["ALPN"] != "h3" || spec.Variables["TLS_VERSION"] != "1.3" {
		t.Fatalf("variables=%v, want h3/TLS1.3", spec.Variables)
	}
	for _, scenario := range spec.Scenarios {
		assertHTTP3MatrixScenario(t, scenario, "epoll")
	}
}

func assertHTTP3MatrixScenario(t *testing.T, scenario Scenario, nettyBackend string) {
	t.Helper()
	command := strings.Join(scenario.Command, " ")
	if scenario.Protocol != "http3" {
		t.Fatalf("scenario=%+v", scenario)
	}
	if !strings.Contains(command, "http3") || !strings.Contains(command, "tls-version") || !strings.Contains(command, "${TLS_VERSION}") || !strings.Contains(command, "${ALPN}") {
		t.Fatalf("command=%q", command)
	}
	if scenario.Framework == "gnalloy" {
		if scenario.Backend != "rfc9000" || !hasScenarioTag(scenario, "parity-harness") {
			t.Fatalf("gnalloy scenario=%+v", scenario)
		}
		if strings.Contains(command, "rfc9000") {
			t.Fatalf("gnalloy HTTP/3 CLI must not pass rfc9000 as a poller backend: %q", command)
		}
		return
	}
	if scenario.Framework == "netty" {
		if scenario.Backend != nettyBackend || !strings.Contains(command, nettyBackend) {
			t.Fatalf("netty scenario=%+v command=%q", scenario, command)
		}
		return
	}
	t.Fatalf("framework=%q, want gnalloy or netty", scenario.Framework)
}

func TestUDPEchoMatrixSpecLoads(t *testing.T) {
	file, err := os.Open("udp-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) != 8 {
		t.Fatalf("scenarios=%d, want udp matrix scenarios", len(spec.Scenarios))
	}
	for _, scenario := range spec.Scenarios {
		command := strings.Join(scenario.Command, " ")
		if scenario.Protocol != "udp-echo" || !strings.Contains(command, "udp-echo") {
			t.Fatalf("scenario=%+v command=%q", scenario, command)
		}
		if scenario.Framework == "netpoll" {
			t.Fatalf("udp matrix must not include netpoll scenario: %+v", scenario)
		}
		if scenario.Warmup != 1 || scenario.Repeat != 3 {
			t.Fatalf("scenario %q warmup=%d repeat=%d, want 1/3", scenario.Name, scenario.Warmup, scenario.Repeat)
		}
	}
}

func TestLinuxUDPEchoMatrixSpecLoads(t *testing.T) {
	file, err := os.Open("linux-udp-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	spec, err := LoadSpec(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) != 8 {
		t.Fatalf("scenarios=%d, want linux udp matrix scenarios", len(spec.Scenarios))
	}
	for _, scenario := range spec.Scenarios {
		command := strings.Join(scenario.Command, " ")
		if scenario.Protocol != "udp-echo" || !strings.Contains(command, "udp-echo") {
			t.Fatalf("scenario=%+v command=%q", scenario, command)
		}
		if scenario.Framework == "netpoll" {
			t.Fatalf("linux udp matrix must not include netpoll scenario: %+v", scenario)
		}
		if scenario.Framework == "gnalloy" && scenario.Backend != "epoll" {
			t.Fatalf("gnalloy backend=%q, want epoll", scenario.Backend)
		}
		if scenario.Framework == "netty" && scenario.Backend != "epoll" {
			t.Fatalf("netty backend=%q, want epoll", scenario.Backend)
		}
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

func TestWindowsTCPMatrixExternalHarnessesCanPassStrictGateWithRepoArtifacts(t *testing.T) {
	file, err := os.Open("windows-tcp.json")
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
				"benchmarks/external/bin/gnet-bench.exe":
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

func TestWindowsHTTP1MatrixExternalHarnessesCanPassStrictGateWithRepoArtifacts(t *testing.T) {
	file, err := os.Open("windows-http1-matrix.json")
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
				"benchmarks/external/bin/gnet-bench.exe":
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

func TestLinuxHTTP1MatrixExternalHarnessesCanPassStrictGateWithRepoArtifacts(t *testing.T) {
	file, err := os.Open("linux-http1-matrix.json")
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
		os.Stdout.WriteString("helper-output\nframework=gnalloy protocol=tcp-echo backend=iocp latencySampleRate=1 latencySamples=6 p50LatencyNs=1000 p99LatencyNs=2000 rssBytes=4096 gcCount=1 payload=1024 connections=2 messages=3 total=6 errors=0 elapsed=2ms throughput=3000.50 ops/s\nBenchmarkEcho-16 1000 123 ns/op 64 B/op 2 allocs/op\n")
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
				{Framework: "netty", Protocol: "tcp", Backend: "epoll", EventLoops: 8, TotalRequests: 100, Errors: 0, Elapsed: 100 * time.Millisecond, ThroughputOpsPerSec: 1000},
				{Framework: "netty", Protocol: "tcp", Backend: "epoll", EventLoops: 8, TotalRequests: 100, Errors: 0, Elapsed: 80 * time.Millisecond, ThroughputOpsPerSec: 1250},
			},
			Metrics: []BenchmarkMetric{{Name: "BenchmarkEcho-16", Iterations: 1000, NsPerOp: 123, BytesPerOp: 64, AllocsPerOp: 2}},
		}},
	}
	var out bytes.Buffer
	if err := WriteReport(&out, report, FormatMarkdown); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"# parity", "| os | linux |", "## Summary", "Throughput median", "P99 latency", "eventLoops=8", "1125", "BenchmarkEcho-16", "### netty", "java -jar bench.jar", "ok"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
}

func TestParseScenarioStats(t *testing.T) {
	stats := ParseScenarioStats("framework=gnalloy protocol=tcp-echo backend=iocp tlsVersion=1.2 cipherSuites=TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 negotiatedProtocol=http/1.1 boss=1 workers=8 readBufferSize=4096 reuseport=true mmap=true mmapBlockSize=4096 mmapBlocks=512 iouringFixedBuffers=true iouringMultishotAccept=true iouringSQPoll=true latencySampleRate=16 latencySamples=1600 p50LatencyNs=1000 p95LatencyNs=1500 p99LatencyNs=2000 p999LatencyNs=3000 maxLatencyNs=4000 rssBytes=4096 heapAllocBytes=2048 heapSysBytes=8192 heapObjects=64 gcCount=2 gcPauseNs=100 goroutines=12 payload=1024 connections=256 messages=100000 total=25600000 errors=0 elapsed=3.2s throughput=8000000.25 ops/s\n")
	if len(stats) != 1 {
		t.Fatalf("stats=%+v, want one", stats)
	}
	stat := stats[0]
	if stat.Framework != "gnalloy" || stat.Protocol != "tcp-echo" || stat.Backend != "iocp" {
		t.Fatalf("stat=%+v", stat)
	}
	if stat.NegotiatedProtocol != "http/1.1" {
		t.Fatalf("negotiatedProtocol=%q, want http/1.1", stat.NegotiatedProtocol)
	}
	if stat.TLSVersion != "1.2" {
		t.Fatalf("tlsVersion=%q, want 1.2", stat.TLSVersion)
	}
	if stat.CipherSuites != "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256" {
		t.Fatalf("cipherSuites=%q, want TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", stat.CipherSuites)
	}
	if stat.PayloadBytes != 1024 || stat.Connections != 256 || stat.Messages != 100000 || stat.TotalRequests != 25600000 || stat.Errors != 0 {
		t.Fatalf("stat=%+v", stat)
	}
	if stat.Boss != 1 || stat.Workers != 8 || stat.ReadBufferBytes != 4096 {
		t.Fatalf("stat=%+v", stat)
	}
	if !stat.ReusePort || !stat.Mmap || !stat.IOUringFixedBuffers || !stat.IOUringMultishotAccept || !stat.IOUringSQPoll {
		t.Fatalf("native flags=%+v", stat)
	}
	if stat.MmapBlockSize != 4096 || stat.MmapBlocks != 512 {
		t.Fatalf("mmap shape=%+v", stat)
	}
	if stat.Elapsed != 3200*time.Millisecond || stat.ThroughputOpsPerSec != 8000000.25 {
		t.Fatalf("stat=%+v", stat)
	}
	if stat.LatencySampleRate != 16 || stat.LatencySamples != 1600 || stat.P50LatencyNanos != 1000 || stat.P99LatencyNanos != 2000 || stat.P999LatencyNanos != 3000 || stat.MaxLatencyNanos != 4000 {
		t.Fatalf("latency=%+v", stat)
	}
	if stat.RSSBytes != 4096 || stat.HeapAllocBytes != 2048 || stat.HeapSysBytes != 8192 || stat.HeapObjects != 64 || stat.GCCount != 2 || stat.GCPauseNanos != 100 || stat.Goroutines != 12 {
		t.Fatalf("resources=%+v", stat)
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

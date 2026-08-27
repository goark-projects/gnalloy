package parity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultScenarioTimeout = 10 * time.Minute

// Runner 执行对标压测规格。
type Runner struct {
	DryRun bool
	Now    func() time.Time
}

// Machine 描述报告中的机器和运行时信息。
type Machine struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	CPUs     int    `json:"cpus"`
	Go       string `json:"go"`
	IPs      string `json:"ips,omitempty"`
}

// Report 是一次对标压测的完整输出。
type Report struct {
	Name       string           `json:"name"`
	Notes      string           `json:"notes,omitempty"`
	StartedAt  time.Time        `json:"startedAt"`
	FinishedAt time.Time        `json:"finishedAt"`
	Machine    Machine          `json:"machine"`
	Scenarios  []ScenarioResult `json:"scenarios"`
}

// ScenarioResult 描述单个压测命令的执行结果。
type ScenarioResult struct {
	Scenario Scenario          `json:"scenario"`
	Started  time.Time         `json:"started"`
	Finished time.Time         `json:"finished"`
	Duration time.Duration     `json:"duration"`
	ExitCode int               `json:"exitCode"`
	Skipped  bool              `json:"skipped"`
	Samples  []ScenarioSample  `json:"samples,omitempty"`
	Stats    []ScenarioStats   `json:"stats,omitempty"`
	Metrics  []BenchmarkMetric `json:"metrics,omitempty"`
	Output   string            `json:"output,omitempty"`
	Error    string            `json:"error,omitempty"`
}

// ScenarioSample 描述一次正式采样命令的原始执行结果。
type ScenarioSample struct {
	Index    int               `json:"index"`
	Started  time.Time         `json:"started"`
	Finished time.Time         `json:"finished"`
	Duration time.Duration     `json:"duration"`
	ExitCode int               `json:"exitCode"`
	Stats    []ScenarioStats   `json:"stats,omitempty"`
	Metrics  []BenchmarkMetric `json:"metrics,omitempty"`
	Output   string            `json:"output,omitempty"`
	Error    string            `json:"error,omitempty"`
}

// Run 执行全部场景并返回报告。单个场景失败不会中止后续场景。
func (r Runner) Run(ctx context.Context, spec Spec) (Report, error) {
	if err := spec.Validate(); err != nil {
		return Report{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := r.now()
	report := Report{
		Name:      spec.Name,
		Notes:     spec.Notes,
		StartedAt: now,
		Machine:   DetectMachine(),
		Scenarios: make([]ScenarioResult, 0, len(spec.Scenarios)),
	}
	for _, original := range spec.Scenarios {
		scenario := expandScenario(original, spec.Variables)
		result := r.runScenario(ctx, scenario)
		report.Scenarios = append(report.Scenarios, result)
	}
	report.FinishedAt = r.now()
	return report, nil
}

func (r Runner) runScenario(ctx context.Context, scenario Scenario) ScenarioResult {
	started := r.now()
	result := ScenarioResult{Scenario: scenario, Started: started}
	if scenario.Skip {
		result.Skipped = true
		result.Output = scenario.SkipReason
		result.Finished = r.now()
		result.Duration = result.Finished.Sub(started)
		return result
	}
	if r.DryRun {
		result.Skipped = true
		result.Finished = r.now()
		result.Duration = result.Finished.Sub(started)
		return result
	}

	for i := 0; i < scenario.Warmup; i++ {
		sample := r.runScenarioCommand(ctx, scenario, 0)
		if sample.ExitCode != 0 || sample.Error != "" {
			result.Finished = r.now()
			result.Duration = result.Finished.Sub(started)
			result.ExitCode = sample.ExitCode
			result.Output = sample.Output
			result.Error = fmt.Sprintf("warmup %d failed: %s", i+1, sample.Error)
			return result
		}
	}

	repeat := scenarioRepeat(scenario)
	if repeat > 1 || scenario.Warmup > 0 {
		result.Samples = make([]ScenarioSample, 0, repeat)
	}
	var output strings.Builder
	for i := 0; i < repeat; i++ {
		sample := r.runScenarioCommand(ctx, scenario, i+1)
		result.Stats = append(result.Stats, sample.Stats...)
		result.Metrics = append(result.Metrics, sample.Metrics...)
		if output.Len() > 0 && sample.Output != "" {
			output.WriteByte('\n')
		}
		output.WriteString(sample.Output)
		if cap(result.Samples) > 0 {
			result.Samples = append(result.Samples, sample)
		}
		if sample.ExitCode != 0 || sample.Error != "" {
			result.ExitCode = sample.ExitCode
			result.Error = sample.Error
			break
		}
	}
	result.Output = output.String()
	result.Finished = r.now()
	result.Duration = result.Finished.Sub(started)
	return result
}

func (r Runner) runScenarioCommand(ctx context.Context, scenario Scenario, index int) ScenarioSample {
	started := r.now()
	sample := ScenarioSample{Index: index, Started: started}
	timeout := time.Duration(scenario.Timeout)
	if timeout == 0 {
		timeout = defaultScenarioTimeout
	}
	scenarioCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	command := resolveScenarioCommand(scenario)
	cmd := exec.CommandContext(scenarioCtx, command, scenario.Command[1:]...)
	if scenario.WorkDir != "" {
		cmd.Dir = scenario.WorkDir
	}
	cmd.Env = mergeEnv(os.Environ(), scenario.Env)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	sample.Output = out.String()
	sample.Stats = ParseScenarioStats(sample.Output)
	sample.Metrics = ParseBenchmarkMetrics(sample.Output)
	sample.Finished = r.now()
	sample.Duration = sample.Finished.Sub(started)
	sample.ExitCode = exitCode(err)
	if err != nil {
		if errors.Is(scenarioCtx.Err(), context.DeadlineExceeded) {
			sample.Error = scenarioCtx.Err().Error()
		} else {
			sample.Error = err.Error()
		}
	}
	return sample
}

func (r Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func resolveScenarioCommand(scenario Scenario) string {
	command := scenario.Command[0]
	trimmed := strings.TrimSpace(command)
	if trimmed == "" || (!hasPathSeparator(trimmed) && !filepath.IsAbs(trimmed)) {
		return command
	}
	resolved, err := pathCommandAvailable(scenario, trimmed, ExternalHarnessOptions{})
	if err != nil {
		return command
	}
	return resolved
}

func scenarioRepeat(scenario Scenario) int {
	if scenario.Repeat <= 0 {
		return 1
	}
	return scenario.Repeat
}

// DetectMachine 采集低风险机器信息，避免读取敏感环境变量。
func DetectMachine() Machine {
	host, _ := os.Hostname()
	return Machine{
		Hostname: host,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPUs:     runtime.NumCPU(),
		Go:       runtime.Version(),
		IPs:      localIPs(),
	}
}

func mergeEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}
	out := append([]string(nil), base...)
	for key, value := range extra {
		if strings.TrimSpace(key) == "" {
			continue
		}
		out = append(out, key+"="+value)
	}
	return out
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func localIPs() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	ips := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
			continue
		}
		if ip := ipNet.IP.To4(); ip != nil {
			ips = append(ips, ip.String())
			continue
		}
		if ip := ipNet.IP.To16(); ip != nil {
			ips = append(ips, ip.String())
		}
	}
	return strings.Join(ips, ",")
}

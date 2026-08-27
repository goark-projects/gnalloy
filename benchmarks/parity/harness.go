package parity

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ExternalHarnessOptions 配置外部对标 harness 的可用性检查。
type ExternalHarnessOptions struct {
	Frameworks []string
	LookPath   func(string) (string, error)
	Stat       func(string) (os.FileInfo, error)
}

// ExternalHarnessReport 汇总正式对标 harness 的就绪状态。
type ExternalHarnessReport struct {
	ExternalScenarios int                    `json:"externalScenarios"`
	Ready             int                    `json:"ready"`
	Skipped           int                    `json:"skipped"`
	Missing           int                    `json:"missing"`
	Issues            []ExternalHarnessIssue `json:"issues,omitempty"`
}

// ExternalHarnessIssue 描述一个未满足严格外部对标合同的场景。
type ExternalHarnessIssue struct {
	Scenario  string `json:"scenario"`
	Framework string `json:"framework"`
	Reason    string `json:"reason"`
}

// ExternalHarnessError 暴露严格外部对标失败的结构化报告。
type ExternalHarnessError struct {
	Report ExternalHarnessReport
}

func (e ExternalHarnessError) Error() string {
	if len(e.Report.Issues) == 0 {
		return ErrExternalHarness.Error()
	}
	return fmt.Sprintf("%v: %s", ErrExternalHarness, e.Report.Issues[0].Reason)
}

func (e ExternalHarnessError) Unwrap() error {
	return ErrExternalHarness
}

// InspectExternalHarnesses 检查 gnalloy、Netty、gnet、netpoll 等正式对标 harness 是否可执行。
func InspectExternalHarnesses(spec Spec, options ExternalHarnessOptions) (ExternalHarnessReport, error) {
	if err := spec.Validate(); err != nil {
		return ExternalHarnessReport{}, err
	}
	frameworks := externalFrameworkSet(options.Frameworks)
	report := ExternalHarnessReport{}
	for _, original := range spec.Scenarios {
		scenario := expandScenario(original, spec.Variables)
		if !isExternalScenario(scenario, frameworks) && !hasScenarioTag(scenario, "parity-harness") {
			continue
		}
		report.ExternalScenarios++
		if scenario.Skip {
			report.Skipped++
			report.Issues = append(report.Issues, ExternalHarnessIssue{
				Scenario:  scenario.Name,
				Framework: scenario.Framework,
				Reason:    fmt.Sprintf("%s is skipped: %s", scenario.Name, scenario.SkipReason),
			})
			continue
		}
		if err := commandAvailable(scenario, options); err != nil {
			report.Missing++
			report.Issues = append(report.Issues, ExternalHarnessIssue{
				Scenario:  scenario.Name,
				Framework: scenario.Framework,
				Reason:    err.Error(),
			})
			continue
		}
		report.Ready++
	}
	return report, nil
}

// ValidateExternalHarnesses 在严格模式下要求全部外部 harness 都处于可执行状态。
func ValidateExternalHarnesses(spec Spec, options ExternalHarnessOptions) error {
	report, err := InspectExternalHarnesses(spec, options)
	if err != nil {
		return err
	}
	if len(report.Issues) > 0 {
		return ExternalHarnessError{Report: report}
	}
	return nil
}

func externalFrameworkSet(frameworks []string) map[string]struct{} {
	if len(frameworks) == 0 {
		frameworks = []string{"netty", "gnet", "netpoll"}
	}
	out := make(map[string]struct{}, len(frameworks))
	for _, framework := range frameworks {
		framework = strings.ToLower(strings.TrimSpace(framework))
		if framework != "" {
			out[framework] = struct{}{}
		}
	}
	return out
}

func isExternalScenario(scenario Scenario, frameworks map[string]struct{}) bool {
	if _, ok := frameworks[strings.ToLower(strings.TrimSpace(scenario.Framework))]; ok {
		return true
	}
	for _, tag := range scenario.Tags {
		if strings.EqualFold(strings.TrimSpace(tag), "external") {
			return true
		}
	}
	return false
}

func hasScenarioTag(scenario Scenario, want string) bool {
	for _, tag := range scenario.Tags {
		if strings.EqualFold(strings.TrimSpace(tag), want) {
			return true
		}
	}
	return false
}

func commandAvailable(scenario Scenario, options ExternalHarnessOptions) error {
	if len(scenario.Command) == 0 || strings.TrimSpace(scenario.Command[0]) == "" {
		return fmt.Errorf("%s has empty command", scenario.Name)
	}
	command := strings.TrimSpace(scenario.Command[0])
	if hasPathSeparator(command) || filepath.IsAbs(command) {
		if _, err := pathCommandAvailable(scenario, command, options); err != nil {
			return err
		}
	} else {
		lookPath := options.LookPath
		if lookPath == nil {
			lookPath = exec.LookPath
		}
		if _, err := lookPath(command); err != nil {
			return fmt.Errorf("%s command %q is not available: %w", scenario.Name, command, err)
		}
	}
	if err := javaJarAvailable(scenario, options); err != nil {
		return err
	}
	return nil
}

func pathCommandAvailable(scenario Scenario, command string, options ExternalHarnessOptions) (string, error) {
	stat := options.Stat
	if stat == nil {
		stat = os.Stat
	}
	for _, candidate := range pathCommandCandidates(command, scenario.WorkDir, runtime.GOOS) {
		info, err := stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s command %q is not available", scenario.Name, command)
}

func javaJarAvailable(scenario Scenario, options ExternalHarnessOptions) error {
	if len(scenario.Command) < 3 {
		return nil
	}
	command := strings.TrimSpace(filepath.Base(scenario.Command[0]))
	if !strings.EqualFold(command, "java") && !strings.EqualFold(command, "java.exe") {
		return nil
	}
	if strings.TrimSpace(scenario.Command[1]) != "-jar" {
		return nil
	}
	jar := strings.TrimSpace(scenario.Command[2])
	if jar == "" {
		return fmt.Errorf("%s jar path is empty", scenario.Name)
	}
	if _, err := pathCommandAvailable(scenario, jar, options); err != nil {
		return fmt.Errorf("%s jar %q is not available", scenario.Name, jar)
	}
	return nil
}

func pathCommandCandidates(command string, workDir string, goos string) []string {
	bases := []string{command}
	if !filepath.IsAbs(command) && strings.TrimSpace(workDir) != "" {
		bases = append([]string{filepath.Join(workDir, command)}, bases...)
	}
	out := make([]string, 0, len(bases)*2)
	seen := make(map[string]struct{}, len(bases)*2)
	add := func(candidate string) {
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	for _, base := range bases {
		add(base)
		if goos == "windows" && filepath.Ext(base) == "" {
			add(base + ".exe")
		}
	}
	return out
}

func hasPathSeparator(command string) bool {
	return strings.ContainsAny(command, `/\`)
}

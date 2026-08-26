package parity

import "strings"

func expandScenario(scenario Scenario, variables map[string]string) Scenario {
	if len(variables) == 0 {
		return scenario
	}
	scenario.Name = expandString(scenario.Name, variables)
	scenario.Framework = expandString(scenario.Framework, variables)
	scenario.Protocol = expandString(scenario.Protocol, variables)
	scenario.Payload = expandString(scenario.Payload, variables)
	scenario.Backend = expandString(scenario.Backend, variables)
	scenario.WorkDir = expandString(scenario.WorkDir, variables)
	scenario.SkipReason = expandString(scenario.SkipReason, variables)
	for i := range scenario.Command {
		scenario.Command[i] = expandString(scenario.Command[i], variables)
	}
	if len(scenario.Env) > 0 {
		env := make(map[string]string, len(scenario.Env))
		for key, value := range scenario.Env {
			env[expandString(key, variables)] = expandString(value, variables)
		}
		scenario.Env = env
	}
	for i := range scenario.Tags {
		scenario.Tags[i] = expandString(scenario.Tags[i], variables)
	}
	return scenario
}

func expandString(value string, variables map[string]string) string {
	out := value
	for key, replacement := range variables {
		out = strings.ReplaceAll(out, "${"+key+"}", replacement)
	}
	return out
}

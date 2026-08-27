package parity

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Spec 描述一次同机对标压测。
type Spec struct {
	Name      string            `json:"name"`
	Notes     string            `json:"notes,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
	Scenarios []Scenario        `json:"scenarios"`
}

// Scenario 描述一个可独立复现的压测命令。
type Scenario struct {
	Name       string            `json:"name"`
	Framework  string            `json:"framework"`
	Protocol   string            `json:"protocol"`
	Payload    string            `json:"payload,omitempty"`
	Backend    string            `json:"backend,omitempty"`
	Warmup     int               `json:"warmup,omitempty"`
	Repeat     int               `json:"repeat,omitempty"`
	WorkDir    string            `json:"workDir,omitempty"`
	Command    []string          `json:"command,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Timeout    Duration          `json:"timeout,omitempty"`
	Tags       []string          `json:"tags,omitempty"`
	Skip       bool              `json:"skip,omitempty"`
	SkipReason string            `json:"skipReason,omitempty"`
}

// Duration 为 JSON 配置提供 time.Duration 文本格式。
type Duration time.Duration

func (d *Duration) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		*d = 0
		return nil
	}
	value, err := time.ParseDuration(text)
	if err != nil {
		return err
	}
	*d = Duration(value)
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// LoadSpec 从 JSON 读取压测规格。
func LoadSpec(r io.Reader) (Spec, error) {
	if r == nil {
		return Spec{}, fmt.Errorf("%w: nil reader", ErrInvalidSpec)
	}
	var spec Spec
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return Spec{}, err
	}
	if err := spec.Validate(); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

// Validate 校验压测规格边界。
func (s Spec) Validate() error {
	if len(s.Scenarios) == 0 {
		return fmt.Errorf("%w: no scenarios", ErrInvalidSpec)
	}
	for i, scenario := range s.Scenarios {
		if err := scenario.Validate(); err != nil {
			return fmt.Errorf("%w %d: %v", ErrInvalidScenario, i, err)
		}
	}
	return nil
}

// Validate 校验单个压测场景。
func (s Scenario) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("empty name")
	}
	if strings.TrimSpace(s.Framework) == "" {
		return fmt.Errorf("empty framework")
	}
	if strings.TrimSpace(s.Protocol) == "" {
		return fmt.Errorf("empty protocol")
	}
	if !s.Skip && (len(s.Command) == 0 || strings.TrimSpace(s.Command[0]) == "") {
		return fmt.Errorf("empty command")
	}
	if time.Duration(s.Timeout) < 0 {
		return fmt.Errorf("negative timeout")
	}
	if s.Warmup < 0 {
		return fmt.Errorf("negative warmup")
	}
	if s.Repeat < 0 {
		return fmt.Errorf("negative repeat")
	}
	return nil
}

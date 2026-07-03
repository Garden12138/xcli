package workflow

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"gopkg.in/yaml.v3"
)

const (
	CurrentVersion  = 1
	DefaultTimeout  = "30m"
	MaxInlineOutput = 128 * 1024
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

type Workflow struct {
	Version int               `yaml:"version" json:"version"`
	Name    string            `yaml:"name" json:"name"`
	Cwd     string            `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	Vars    map[string]string `yaml:"vars,omitempty" json:"vars,omitempty"`
	Steps   []Step            `yaml:"steps" json:"steps"`
}

type Step struct {
	ID              string   `yaml:"id" json:"id"`
	Agent           string   `yaml:"agent" json:"agent"`
	Prompt          string   `yaml:"prompt" json:"prompt"`
	Args            []string `yaml:"args,omitempty" json:"args,omitempty"`
	Cwd             string   `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	Network         string   `yaml:"network,omitempty" json:"network,omitempty"`
	DependsOn       []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Timeout         string   `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Retries         int      `yaml:"retries,omitempty" json:"retries,omitempty"`
	ContinueOnError bool     `yaml:"continue_on_error,omitempty" json:"continue_on_error,omitempty"`
}

func Load(path string) (Workflow, string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Workflow{}, "", fmt.Errorf("resolve workflow path: %w", err)
	}
	f, err := os.Open(absolute)
	if err != nil {
		return Workflow{}, absolute, err
	}
	defer f.Close()

	var workflow Workflow
	decoder := yaml.NewDecoder(f)
	decoder.KnownFields(true)
	if err := decoder.Decode(&workflow); err != nil {
		return Workflow{}, absolute, fmt.Errorf("decode workflow: %w", err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Workflow{}, absolute, errors.New("multiple YAML documents are not supported")
		}
		return Workflow{}, absolute, fmt.Errorf("decode workflow: %w", err)
	}
	return workflow, absolute, nil
}

func Validate(workflow Workflow, registry *agent.Registry, networks map[string]struct{}) error {
	if workflow.Version != CurrentVersion {
		return fmt.Errorf("unsupported workflow version %d (expected %d)", workflow.Version, CurrentVersion)
	}
	if workflow.Name == "" {
		return errors.New("workflow name is required")
	}
	if len(workflow.Steps) == 0 {
		return errors.New("workflow must contain at least one step")
	}
	for name := range workflow.Vars {
		if !idPattern.MatchString(name) {
			return fmt.Errorf("invalid workflow variable name %q", name)
		}
	}
	seen := make(map[string]bool, len(workflow.Steps))
	for index, step := range workflow.Steps {
		label := fmt.Sprintf("step %d", index+1)
		if !idPattern.MatchString(step.ID) {
			return fmt.Errorf("%s has invalid id %q", label, step.ID)
		}
		if seen[step.ID] {
			return fmt.Errorf("duplicate step id %q", step.ID)
		}
		if !registry.Has(step.Agent) {
			return fmt.Errorf("step %q references unknown agent %q", step.ID, step.Agent)
		}
		if strings.TrimSpace(step.Prompt) == "" {
			return fmt.Errorf("step %q has an empty prompt", step.ID)
		}
		if err := validateTemplateSyntax(step.Prompt, workflow.Vars, seen); err != nil {
			return fmt.Errorf("step %q prompt: %w", step.ID, err)
		}
		for _, arg := range step.Args {
			if err := validateTemplateSyntax(arg, workflow.Vars, seen); err != nil {
				return fmt.Errorf("step %q argument: %w", step.ID, err)
			}
		}
		for _, dependency := range step.DependsOn {
			if !seen[dependency] {
				return fmt.Errorf("step %q depends on %q, which must be declared earlier", step.ID, dependency)
			}
		}
		if step.Retries < 0 {
			return fmt.Errorf("step %q retries cannot be negative", step.ID)
		}
		timeout := step.Timeout
		if timeout == "" {
			timeout = DefaultTimeout
		}
		if _, err := time.ParseDuration(timeout); err != nil {
			return fmt.Errorf("step %q has invalid timeout %q: %w", step.ID, timeout, err)
		}
		if step.Network != "" {
			if _, ok := networks[step.Network]; !ok {
				return fmt.Errorf("step %q references unknown network %q", step.ID, step.Network)
			}
		}
		seen[step.ID] = true
	}
	return nil
}

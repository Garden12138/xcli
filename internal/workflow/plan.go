package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	AvailabilityOK      = "ok"
	AvailabilityMissing = "missing"
	AvailabilityRuntime = "runtime"
)

type Plan struct {
	SourcePath       string        `json:"source_path"`
	Name             string        `json:"name"`
	WorkingDirectory string        `json:"working_directory"`
	MaxParallel      int           `json:"max_parallel"`
	WorkspaceMode    string        `json:"workspace_mode,omitempty"`
	Variables        []string      `json:"variables,omitempty"`
	Applicable       bool          `json:"applicable"`
	Errors           []string      `json:"errors,omitempty"`
	Steps            []PlannedStep `json:"steps"`
}

type PlannedStep struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Agent           string   `json:"agent,omitempty"`
	Command         string   `json:"command,omitempty"`
	Executable      string   `json:"executable,omitempty"`
	Availability    string   `json:"availability"`
	Cwd             string   `json:"cwd"`
	Network         string   `json:"network,omitempty"`
	DependsOn       []string `json:"depends_on,omitempty"`
	Timeout         string   `json:"timeout"`
	Retries         int      `json:"retries"`
	ContinueOnError bool     `json:"continue_on_error"`
	Error           string   `json:"error,omitempty"`
}

func (r *Runner) BuildPlan(prepared PreparedRun) Plan {
	plan := Plan{
		SourcePath: prepared.SourcePath, Name: prepared.Definition.Name,
		WorkingDirectory: prepared.WorkingDirectory, MaxParallel: prepared.MaxParallel,
		Applicable: true, Steps: make([]PlannedStep, 0, len(prepared.Definition.Steps)),
	}
	for name := range prepared.Variables {
		plan.Variables = append(plan.Variables, name)
	}
	sort.Strings(plan.Variables)

	templateData := templateContext{Vars: prepared.Variables, Steps: map[string]templateStep{}}
	for _, step := range prepared.Definition.Steps {
		item := PlannedStep{
			ID: step.ID, Agent: step.Agent, Network: step.Network,
			DependsOn: effectiveDependencies(step), Timeout: step.Timeout,
			Retries: step.Retries, ContinueOnError: step.ContinueOnError,
		}
		if item.Timeout == "" {
			item.Timeout = DefaultTimeout
		}
		directory, err := resolveDirectory(prepared.WorkingDirectory, step.Cwd)
		if err != nil {
			item.Availability = AvailabilityMissing
			item.Error = err.Error()
			plan.Applicable = false
			plan.Steps = append(plan.Steps, item)
			continue
		}
		item.Cwd = directory

		if step.Command != "" {
			item.Kind = "command"
			item.Command = step.Command
			if len(templatePattern.FindAllStringSubmatch(step.Command, -1)) > 0 &&
				len(stepTemplateResultReferences(step.Command)) > 0 {
				item.Availability = AvailabilityRuntime
				plan.Steps = append(plan.Steps, item)
				continue
			}
			resolved, resolveErr := resolveTemplate(step.Command, templateData)
			if resolveErr != nil {
				item.Availability = AvailabilityMissing
				item.Error = resolveErr.Error()
				plan.Applicable = false
				plan.Steps = append(plan.Steps, item)
				continue
			}
			item.Command = resolved
			item.Executable, err = lookCommand(resolved, directory)
		} else {
			item.Kind = "agent"
			definition, getErr := r.Registry.Get(step.Agent)
			if getErr != nil {
				err = getErr
			} else {
				item.Command = definition.Config.Command
				item.Executable, err = exec.LookPath(definition.Config.Command)
			}
		}
		if err != nil {
			item.Availability = AvailabilityMissing
			item.Error = err.Error()
			plan.Applicable = false
		} else {
			item.Availability = AvailabilityOK
		}
		plan.Steps = append(plan.Steps, item)
	}
	return plan
}

func stepTemplateResultReferences(value string) []string {
	seen := map[string]bool{}
	var result []string
	for _, match := range templatePattern.FindAllStringSubmatch(value, -1) {
		parts := strings.Split(match[1], ".")
		if len(parts) == 3 && parts[0] == "steps" && !seen[parts[1]] {
			seen[parts[1]] = true
			result = append(result, parts[1])
		}
	}
	return result
}

func lookCommand(name, directory string) (string, error) {
	if !strings.ContainsRune(name, os.PathSeparator) {
		return exec.LookPath(name)
	}
	path := name
	if !filepath.IsAbs(path) {
		path = filepath.Join(directory, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("command %q is not available: %w", name, err)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("command %q is not executable", name)
	}
	return filepath.Clean(path), nil
}

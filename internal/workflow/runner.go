package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/runstore"
	xruntime "github.com/Garden12138/xcli/internal/runtime"
)

type StepResult struct {
	ID         string    `json:"id"`
	Agent      string    `json:"agent"`
	Status     string    `json:"status"`
	Output     string    `json:"output,omitempty"`
	OutputFile string    `json:"output_file,omitempty"`
	SessionID  string    `json:"session_id,omitempty"`
	ExitCode   int       `json:"exit_code"`
	Attempts   int       `json:"attempts"`
	Error      string    `json:"error,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	EndedAt    time.Time `json:"ended_at,omitempty"`
}

type Execution struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Status   string       `json:"status"`
	ExitCode int          `json:"exit_code"`
	Cwd      string       `json:"cwd"`
	Steps    []StepResult `json:"steps"`
}

type Runner struct {
	Config   config.Config
	Registry *agent.Registry
	Store    *runstore.Store
	Progress io.Writer
	Stderr   io.Writer
}

func (r *Runner) Run(ctx context.Context, sourcePath string, workflow Workflow, overrides map[string]string, recordOutput bool) (Execution, error) {
	networks := make(map[string]struct{}, len(r.Config.Networks))
	for name := range r.Config.Networks {
		networks[name] = struct{}{}
	}
	if err := Validate(workflow, r.Registry, networks); err != nil {
		return Execution{}, err
	}

	workingDirectory, err := resolveDirectory(filepath.Dir(sourcePath), workflow.Cwd)
	if err != nil {
		return Execution{}, err
	}
	variables := make(map[string]string, len(workflow.Vars)+len(overrides))
	for key, value := range workflow.Vars {
		variables[key] = value
	}
	for key, value := range overrides {
		if _, ok := workflow.Vars[key]; !ok {
			return Execution{}, fmt.Errorf("cannot override undeclared workflow variable %q", key)
		}
		variables[key] = value
	}

	runID := runstore.NewID("workflow")
	execution := Execution{ID: runID, Name: workflow.Name, Status: "success", Cwd: workingDirectory}
	record := runstore.Record{
		ID: runID, Kind: "workflow", Workflow: workflow.Name, Cwd: workingDirectory,
		StartedAt: time.Now().UTC(), Status: "running",
	}

	tempDir, err := os.MkdirTemp("", "xcli-workflow-*")
	if err != nil {
		return Execution{}, fmt.Errorf("create workflow output directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	templateData := templateContext{Vars: variables, Steps: map[string]templateStep{}}
	statuses := map[string]string{}
	stop := false
	for _, step := range workflow.Steps {
		if stop {
			result := skippedStep(step, "workflow stopped after a failed step")
			execution.Steps = append(execution.Steps, result)
			statuses[step.ID] = result.Status
			continue
		}
		if ctx.Err() != nil {
			result := skippedStep(step, "workflow canceled")
			result.Status = "canceled"
			result.ExitCode = 130
			execution.Steps = append(execution.Steps, result)
			statuses[step.ID] = result.Status
			execution.Status = "canceled"
			execution.ExitCode = 130
			stop = true
			continue
		}
		if dependency := failedDependency(step.DependsOn, statuses); dependency != "" {
			result := skippedStep(step, fmt.Sprintf("dependency %q did not succeed", dependency))
			execution.Steps = append(execution.Steps, result)
			statuses[step.ID] = result.Status
			execution.Status = "failed"
			execution.ExitCode = 1
			continue
		}

		result := r.runStep(ctx, workingDirectory, tempDir, step, templateData, recordOutput, runID)
		execution.Steps = append(execution.Steps, result)
		statuses[step.ID] = result.Status
		if result.Status == "success" {
			templateData.Steps[step.ID] = templateStep{Output: result.Output, OutputFile: result.OutputFile, SessionID: result.SessionID}
			continue
		}

		execution.Status = result.Status
		execution.ExitCode = result.ExitCode
		if execution.ExitCode == 0 {
			execution.ExitCode = 1
		}
		if !step.ContinueOnError || result.Status == "canceled" || result.Status == "timed_out" {
			stop = true
		}
	}

	if execution.Status == "success" {
		execution.ExitCode = 0
	}
	record.EndedAt = time.Now().UTC()
	record.Status = execution.Status
	record.ExitCode = execution.ExitCode
	for _, step := range execution.Steps {
		outputFile := ""
		if recordOutput {
			outputFile = step.OutputFile
		}
		record.Steps = append(record.Steps, runstore.StepRecord{
			ID: step.ID, Agent: step.Agent, Status: step.Status, ExitCode: step.ExitCode,
			SessionID: step.SessionID, OutputFile: outputFile, StartedAt: step.StartedAt,
			EndedAt: step.EndedAt, Attempts: step.Attempts, Error: step.Error,
		})
	}
	if !recordOutput {
		for index := range execution.Steps {
			execution.Steps[index].OutputFile = ""
		}
	}
	if err := r.Store.Save(record); err != nil {
		return execution, err
	}
	return execution, nil
}

func (r *Runner) runStep(ctx context.Context, workflowCwd, tempDir string, step Step, templateData templateContext, recordOutput bool, runID string) (result StepResult) {
	result = StepResult{ID: step.ID, Agent: step.Agent, Status: "failed", ExitCode: 1, StartedAt: time.Now().UTC()}
	defer func() { result.EndedAt = time.Now().UTC() }()

	prompt, err := resolveTemplate(step.Prompt, templateData)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	args := make([]string, len(step.Args))
	for i, value := range step.Args {
		args[i], err = resolveTemplate(value, templateData)
		if err != nil {
			result.Error = err.Error()
			return result
		}
	}
	directory, err := resolveDirectory(workflowCwd, step.Cwd)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	definition, err := r.Registry.Get(step.Agent)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	spec, err := definition.Run(prompt, true, args)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	environment, err := xruntime.BuildEnvironment(os.Environ(), r.Config, definition.Config, step.Network)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	timeoutText := step.Timeout
	if timeoutText == "" {
		timeoutText = DefaultTimeout
	}
	timeout, _ := time.ParseDuration(timeoutText)
	for attempt := 1; attempt <= step.Retries+1; attempt++ {
		result.Attempts = attempt
		if r.Progress != nil {
			fmt.Fprintf(r.Progress, "[%d/%d] %s (%s)\n", attempt, step.Retries+1, step.ID, step.Agent)
		}
		stepContext := ctx
		cancel := func() {}
		if timeout > 0 {
			stepContext, cancel = context.WithTimeout(ctx, timeout)
		}
		processResult, runErr := xruntime.RunProcess(stepContext, spec, xruntime.ProcessOptions{
			Dir: directory, Env: environment, Stderr: r.Stderr, CaptureStdout: true, SeparateProcess: true,
		})
		cancel()
		if runErr != nil {
			result.Error = runErr.Error()
			continue
		}
		parsed := agent.ParseStructured(definition.Config.Adapter, definition.Config.Output, processResult.Stdout)
		result.Output = parsed.Output
		result.SessionID = parsed.SessionID
		result.ExitCode = processResult.ExitCode
		outputPath := filepath.Join(tempDir, step.ID+".output")
		if err := os.WriteFile(outputPath, []byte(result.Output), 0o600); err != nil {
			result.Error = fmt.Sprintf("write temporary step output: %v", err)
			continue
		}
		result.OutputFile = outputPath
		if recordOutput {
			persistent, err := r.Store.SaveOutput(runID, step.ID+".output", []byte(result.Output))
			if err != nil {
				result.Error = err.Error()
				continue
			}
			if _, err := r.Store.SaveOutput(runID, step.ID+".raw.log", processResult.Stdout); err != nil {
				result.Error = err.Error()
				continue
			}
			result.OutputFile = persistent
		}
		if processResult.TimedOut {
			result.Status = "timed_out"
			result.Error = fmt.Sprintf("step exceeded timeout %s", timeoutText)
		} else if processResult.Canceled {
			result.Status = "canceled"
			result.Error = "workflow canceled"
		} else if processResult.ExitCode == 0 {
			result.Status = "success"
			result.Error = ""
			return result
		} else {
			result.Status = "failed"
			result.Error = fmt.Sprintf("agent exited with code %d", processResult.ExitCode)
		}
		if result.Status == "canceled" || result.Status == "timed_out" {
			return result
		}
	}
	return result
}

func resolveDirectory(base, value string) (string, error) {
	if value == "" {
		value = "."
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(base, value)
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("working directory %s: %w", absolute, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("working directory %s is not a directory", absolute)
	}
	return absolute, nil
}

func failedDependency(dependencies []string, statuses map[string]string) string {
	for _, dependency := range dependencies {
		if statuses[dependency] != "success" {
			return dependency
		}
	}
	return ""
}

func skippedStep(step Step, reason string) StepResult {
	return StepResult{ID: step.ID, Agent: step.Agent, Status: "skipped", ExitCode: 1, Error: reason}
}

func ParseVars(values []string) (map[string]string, error) {
	result := make(map[string]string, len(values))
	for _, value := range values {
		index := strings.IndexByte(value, '=')
		if index <= 0 {
			return nil, fmt.Errorf("invalid variable %q; expected key=value", value)
		}
		key := value[:index]
		if !idPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid variable name %q", key)
		}
		result[key] = value[index+1:]
	}
	return result, nil
}

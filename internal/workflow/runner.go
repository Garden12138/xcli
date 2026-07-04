package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Status      string       `json:"status"`
	ExitCode    int          `json:"exit_code"`
	MaxParallel int          `json:"max_parallel"`
	Cwd         string       `json:"cwd"`
	Steps       []StepResult `json:"steps"`
}

type RunOptions struct {
	VariableOverrides map[string]string
	RecordOutput      bool
	MaxParallel       *int
}

type Runner struct {
	Config   config.Config
	Registry *agent.Registry
	Store    *runstore.Store
	Progress io.Writer
	Stderr   io.Writer
}

type stepState uint8

const (
	stepPending stepState = iota
	stepRunning
	stepFinished
)

type stepCompletion struct {
	index  int
	result StepResult
}

type lockedWriter struct {
	mu     *sync.Mutex
	writer io.Writer
}

func (w *lockedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(data)
}

func (r *Runner) Run(ctx context.Context, sourcePath string, workflow Workflow, options RunOptions) (Execution, error) {
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
	variables := make(map[string]string, len(workflow.Vars)+len(options.VariableOverrides))
	for key, value := range workflow.Vars {
		variables[key] = value
	}
	for key, value := range options.VariableOverrides {
		if _, ok := workflow.Vars[key]; !ok {
			return Execution{}, fmt.Errorf("cannot override undeclared workflow variable %q", key)
		}
		variables[key] = value
	}
	maxParallel := DefaultMaxParallel
	if workflow.MaxParallel != nil {
		maxParallel = *workflow.MaxParallel
	}
	if options.MaxParallel != nil {
		if *options.MaxParallel <= 0 {
			return Execution{}, errors.New("max parallel must be greater than zero")
		}
		maxParallel = *options.MaxParallel
	}

	runID := runstore.NewID("workflow")
	execution := Execution{
		ID: runID, Name: workflow.Name, Status: "success", MaxParallel: maxParallel,
		Cwd: workingDirectory, Steps: make([]StepResult, len(workflow.Steps)),
	}
	record := runstore.Record{
		ID: runID, Kind: "workflow", Workflow: workflow.Name, MaxParallel: maxParallel, Cwd: workingDirectory,
		StartedAt: time.Now().UTC(), Status: "running",
	}

	tempDir, err := os.MkdirTemp("", "xcli-workflow-*")
	if err != nil {
		return Execution{}, fmt.Errorf("create workflow output directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	stepRunner := *r
	writerMutex := &sync.Mutex{}
	if r.Progress != nil {
		stepRunner.Progress = &lockedWriter{mu: writerMutex, writer: r.Progress}
	}
	if r.Stderr != nil {
		stepRunner.Stderr = &lockedWriter{mu: writerMutex, writer: r.Stderr}
	}

	dependencies := make([][]string, len(workflow.Steps))
	for index, step := range workflow.Steps {
		dependencies[index] = effectiveDependencies(step)
	}
	templateData := templateContext{Vars: variables, Steps: map[string]templateStep{}}
	statuses := make(map[string]string, len(workflow.Steps))
	states := make([]stepState, len(workflow.Steps))
	completed := make(chan stepCompletion, len(workflow.Steps))
	workflowContext, cancelWorkflow := context.WithCancel(ctx)
	defer cancelWorkflow()

	running := 0
	finished := 0
	stopping := false
	externalCanceled := false
	fatalIndex := -1
	stopReason := ""
	externalDone := ctx.Done()

	for finished < len(workflow.Steps) {
		if !stopping && ctx.Err() != nil {
			stopping = true
			externalCanceled = true
			stopReason = "workflow canceled"
			externalDone = nil
			cancelWorkflow()
		}

		if !stopping {
			for index, step := range workflow.Steps {
				if running >= maxParallel {
					break
				}
				if states[index] != stepPending {
					continue
				}
				ready, failedDependency := dependencyReadiness(dependencies[index], statuses)
				if failedDependency != "" {
					result := skippedStep(step, fmt.Sprintf("dependency %q did not succeed", failedDependency))
					execution.Steps[index] = result
					states[index] = stepFinished
					statuses[step.ID] = result.Status
					finished++
					continue
				}
				if !ready {
					continue
				}

				states[index] = stepRunning
				running++
				stepContext := copyTemplateContext(templateData)
				go func(index int, step Step, data templateContext) {
					result := stepRunner.runStep(
						workflowContext, workingDirectory, tempDir, step, data,
						options.RecordOutput, runID,
					)
					completed <- stepCompletion{index: index, result: result}
				}(index, step, stepContext)
			}
		}

		if stopping {
			for index, step := range workflow.Steps {
				if states[index] != stepPending {
					continue
				}
				result := skippedStep(step, stopReason)
				execution.Steps[index] = result
				states[index] = stepFinished
				statuses[step.ID] = result.Status
				finished++
			}
		}

		if finished == len(workflow.Steps) {
			break
		}
		if running == 0 {
			return execution, errors.New("workflow scheduler stalled with pending steps")
		}

		select {
		case completion := <-completed:
			index := completion.index
			result := completion.result
			running--
			finished++
			states[index] = stepFinished
			if stopping && result.Status == "canceled" && index != fatalIndex {
				result.Error = stopReason
			}
			execution.Steps[index] = result
			step := workflow.Steps[index]
			statuses[step.ID] = result.Status
			if result.Status == "success" {
				templateData.Steps[step.ID] = templateStep{
					Output: result.Output, OutputFile: result.OutputFile, SessionID: result.SessionID,
				}
				continue
			}
			if !stopping && isFatalResult(step, result) {
				stopping = true
				fatalIndex = index
				stopReason = fmt.Sprintf("workflow stopped after step %q %s", step.ID, result.Status)
				externalDone = nil
				cancelWorkflow()
			}
		case <-externalDone:
			stopping = true
			externalCanceled = true
			stopReason = "workflow canceled"
			externalDone = nil
			cancelWorkflow()
		}
	}

	summarizeExecution(&execution, externalCanceled, fatalIndex)
	record.EndedAt = time.Now().UTC()
	record.Status = execution.Status
	record.ExitCode = execution.ExitCode
	for _, step := range execution.Steps {
		outputFile := ""
		if options.RecordOutput {
			outputFile = step.OutputFile
		}
		record.Steps = append(record.Steps, runstore.StepRecord{
			ID: step.ID, Agent: step.Agent, Status: step.Status, ExitCode: step.ExitCode,
			SessionID: step.SessionID, OutputFile: outputFile, StartedAt: step.StartedAt,
			EndedAt: step.EndedAt, Attempts: step.Attempts, Error: step.Error,
		})
	}
	if !options.RecordOutput {
		for index := range execution.Steps {
			execution.Steps[index].OutputFile = ""
		}
	}
	if err := r.Store.Save(record); err != nil {
		return execution, err
	}
	return execution, nil
}

func effectiveDependencies(step Step) []string {
	seen := make(map[string]bool, len(step.DependsOn))
	dependencies := make([]string, 0, len(step.DependsOn))
	for _, dependency := range append(append([]string{}, step.DependsOn...), stepTemplateReferences(step)...) {
		if seen[dependency] {
			continue
		}
		seen[dependency] = true
		dependencies = append(dependencies, dependency)
	}
	return dependencies
}

func dependencyReadiness(dependencies []string, statuses map[string]string) (bool, string) {
	waiting := false
	for _, dependency := range dependencies {
		status, finished := statuses[dependency]
		if !finished {
			waiting = true
			continue
		}
		if status != "success" {
			return false, dependency
		}
	}
	return !waiting, ""
}

func copyTemplateContext(source templateContext) templateContext {
	result := templateContext{
		Vars:  make(map[string]string, len(source.Vars)),
		Steps: make(map[string]templateStep, len(source.Steps)),
	}
	for key, value := range source.Vars {
		result.Vars[key] = value
	}
	for key, value := range source.Steps {
		result.Steps[key] = value
	}
	return result
}

func isFatalResult(step Step, result StepResult) bool {
	return !step.ContinueOnError || result.Status == "canceled" || result.Status == "timed_out"
}

func summarizeExecution(execution *Execution, externalCanceled bool, fatalIndex int) {
	if externalCanceled {
		execution.Status = "canceled"
		execution.ExitCode = 130
		return
	}
	if fatalIndex >= 0 {
		result := execution.Steps[fatalIndex]
		execution.Status = result.Status
		execution.ExitCode = result.ExitCode
		if execution.ExitCode == 0 {
			execution.ExitCode = 1
		}
		return
	}
	for _, result := range execution.Steps {
		if result.Status == "success" || result.Status == "skipped" {
			continue
		}
		execution.Status = result.Status
		execution.ExitCode = result.ExitCode
		if execution.ExitCode == 0 {
			execution.ExitCode = 1
		}
		return
	}
	execution.Status = "success"
	execution.ExitCode = 0
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

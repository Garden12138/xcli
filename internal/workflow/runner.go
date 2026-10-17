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
	"github.com/Garden12138/xcli/internal/worktree"
)

type StepResult struct {
	RetryNotices int          `json:"retry_notices,omitempty"`
	Diagnostics  string       `json:"-"`
	Cwd          string       `json:"cwd,omitempty"`
	ParseStatus  string       `json:"parse_status,omitempty"`
	ErrorKind    string       `json:"error_kind,omitempty"`
	CLIExitCode  int          `json:"-"`
	ID           string       `json:"id"`
	Agent        string       `json:"agent,omitempty"`
	AgentVersion string       `json:"-"`
	Command      string       `json:"command,omitempty"`
	Status       string       `json:"status"`
	Output       string       `json:"output,omitempty"`
	OutputFile   string       `json:"output_file,omitempty"`
	SessionID    string       `json:"session_id,omitempty"`
	Usage        *agent.Usage `json:"usage,omitempty"`
	ExitCode     int          `json:"exit_code"`
	Attempts     int          `json:"attempts"`
	Error        string       `json:"error,omitempty"`
	StartedAt    time.Time    `json:"started_at,omitempty"`
	EndedAt      time.Time    `json:"ended_at,omitempty"`
}

type Execution struct {
	RunID       string       `json:"run_id,omitempty"`
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Status      string       `json:"status"`
	ExitCode    int          `json:"exit_code"`
	MaxParallel int          `json:"max_parallel"`
	Cwd         string       `json:"cwd"`
	Steps       []StepResult `json:"steps"`
	Usage       *agent.Usage `json:"usage,omitempty"`
}

type RunOptions struct {
	Worktree          bool
	VariableOverrides map[string]string
	RecordOutput      bool
	MaxParallel       *int
}

type PreparedRun struct {
	ID               string             `json:"id,omitempty"`
	StepCwds         map[string]string  `json:"step_cwds"`
	Workspace        *worktree.Metadata `json:"workspace,omitempty"`
	SourceCwd        string             `json:"source_cwd,omitempty"`
	SourcePath       string             `json:"source_path"`
	Definition       Workflow           `json:"definition"`
	WorkingDirectory string             `json:"working_directory"`
	Variables        map[string]string  `json:"variables,omitempty"`
	RecordOutput     bool               `json:"record_output,omitempty"`
	MaxParallel      int                `json:"max_parallel"`
}

type Runner struct {
	literal       bool
	Events        func(agent.Event) error
	workspaceRoot string
	Config        config.Config
	Registry      *agent.Registry
	Store         *runstore.Store
	Progress      io.Writer
	Stderr        io.Writer
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
	prepared, err := r.Prepare(sourcePath, workflow, options)
	if err != nil {
		return Execution{}, err
	}
	return r.RunPrepared(ctx, prepared, nil)
}

func (r *Runner) Prepare(sourcePath string, workflow Workflow, options RunOptions) (PreparedRun, error) {
	networks := make(map[string]struct{}, len(r.Config.Networks))
	for name := range r.Config.Networks {
		networks[name] = struct{}{}
	}
	if err := Validate(workflow, r.Registry, networks); err != nil {
		return PreparedRun{}, err
	}

	workingDirectory, err := resolveDirectory(filepath.Dir(sourcePath), workflow.Cwd)
	if err != nil {
		return PreparedRun{}, err
	}
	variables := make(map[string]string, len(workflow.Vars)+len(options.VariableOverrides))
	for key, value := range workflow.Vars {
		variables[key] = value
	}
	for key, value := range options.VariableOverrides {
		if _, ok := workflow.Vars[key]; !ok {
			return PreparedRun{}, fmt.Errorf("cannot override undeclared workflow variable %q", key)
		}
		variables[key] = value
	}
	maxParallel := DefaultMaxParallel
	if workflow.MaxParallel != nil {
		maxParallel = *workflow.MaxParallel
	}
	if options.MaxParallel != nil {
		if *options.MaxParallel <= 0 {
			return PreparedRun{}, errors.New("max parallel must be greater than zero")
		}
		maxParallel = *options.MaxParallel
	}
	prepared := PreparedRun{
		ID: runstore.NewID("workflow"), SourcePath: sourcePath, Definition: workflow, WorkingDirectory: workingDirectory,
		SourceCwd: workingDirectory, Variables: variables, RecordOutput: options.RecordOutput, MaxParallel: maxParallel, StepCwds: map[string]string{},
	}
	for _, step := range workflow.Steps {
		directory, err := resolveDirectory(workingDirectory, step.Cwd)
		if err != nil {
			return PreparedRun{}, fmt.Errorf("step %q: %w", step.ID, err)
		}
		prepared.StepCwds[step.ID] = directory
	}
	if options.Worktree {
		if prepared.MaxParallel > 1 {
			return PreparedRun{}, errors.New("isolated workflows allow only one writable role; set max_parallel to 1")
		}
		manager, err := worktree.New()
		if err != nil {
			return PreparedRun{}, err
		}
		metadata, err := manager.Preview(prepared.ID, workingDirectory)
		if err != nil {
			return PreparedRun{}, err
		}
		prepared.Workspace = &metadata
		prepared.WorkingDirectory, err = worktree.PlannedPath(metadata, workingDirectory)
		if err != nil {
			return PreparedRun{}, err
		}
		for id, directory := range prepared.StepCwds {
			mapped, err := worktree.PlannedPath(metadata, directory)
			if err != nil {
				return PreparedRun{}, fmt.Errorf("step %q: %w", id, err)
			}
			prepared.StepCwds[id] = mapped
		}
	}
	return prepared, nil
}

func (r *Runner) RunPrepared(ctx context.Context, prepared PreparedRun, initial *runstore.Record) (Execution, error) {
	workflow := prepared.Definition
	versionDetectionStarted := time.Now()
	agentVersions := make(map[string]string)
	versionChecks := make(map[string]bool)
	type detectedVersion struct {
		agent   string
		version string
	}
	versionResults := make(chan detectedVersion, len(workflow.Steps))
	for _, step := range workflow.Steps {
		if step.Agent == "" {
			continue
		}
		if versionChecks[step.Agent] {
			continue
		}
		versionChecks[step.Agent] = true
		definition, err := r.Registry.Get(step.Agent)
		if err != nil {
			continue
		}
		go func(name string, definition agent.Definition) {
			versionResults <- detectedVersion{agent: name, version: definition.Detect(ctx).Version}
		}(step.Agent, definition)
	}
	runID := prepared.ID
	if runID == "" {
		runID = runstore.NewID("workflow")
	}
	startedAt := time.Now().UTC()
	record := runstore.Record{}
	liveProgress := true
	if initial != nil {
		record = *initial
		runID = record.ID
		startedAt = record.StartedAt
	}
	execution := Execution{
		ID: runID, RunID: runID, Name: workflow.Name, Status: "success", MaxParallel: prepared.MaxParallel,
		Cwd: prepared.WorkingDirectory, Steps: make([]StepResult, len(workflow.Steps)),
	}
	for index, step := range workflow.Steps {
		execution.Steps[index] = StepResult{
			ID: step.ID, Agent: step.Agent, AgentVersion: agentVersions[step.Agent],
			Command: step.Command, Cwd: prepared.StepCwds[step.ID], Status: "pending",
		}
	}
	record.ID = runID
	record.Kind = "workflow"
	record.Workflow = workflow.Name
	record.WorkflowFile = prepared.SourcePath
	record.MaxParallel = prepared.MaxParallel
	record.Cwd = prepared.WorkingDirectory
	record.StartedAt = startedAt
	record.Status = "running"
	record.AgentVersions = agentVersions
	if prepared.Workspace != nil {
		metadata := *prepared.Workspace
		manager, err := worktree.New()
		if err != nil {
			return execution, err
		}
		if initial == nil || !initial.Background {
			created, err := manager.Create(metadata.RunID, prepared.SourceCwd)
			if err != nil {
				return execution, err
			}
			if created.BaseCommit != metadata.BaseCommit {
				return execution, errors.New("repository baseline changed after preparation")
			}
		}
		if err := worktree.CheckDirectory(metadata.Path, prepared.WorkingDirectory); err != nil {
			return execution, err
		}
		for _, directory := range prepared.StepCwds {
			if err := worktree.CheckDirectory(metadata.Path, directory); err != nil {
				return execution, err
			}
		}
		record.WorkspaceMode = "worktree"
		record.Repository = metadata.Repository
		record.WorktreePath = metadata.Path
		record.WorktreeBranch = metadata.Branch
		record.BaseCommit = metadata.BaseCommit
	}
	record.Steps = executionStepRecords(execution.Steps, prepared.RecordOutput)
	if liveProgress {
		if err := r.Store.Save(record); err != nil {
			return execution, err
		}
	}

	tempDir, err := os.MkdirTemp("", "xcli-workflow-*")
	if err != nil {
		return Execution{}, fmt.Errorf("create workflow output directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	stepRunner := *r
	if prepared.Workspace != nil {
		stepRunner.workspaceRoot = prepared.Workspace.Path
	}
	writerMutex := &sync.Mutex{}
	if r.Events != nil {
		stepRunner.Events = func(event agent.Event) error { writerMutex.Lock(); defer writerMutex.Unlock(); return r.Events(event) }
	}
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
	templateData := templateContext{Vars: prepared.Variables, Steps: map[string]templateStep{}}
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
	progressErr := error(nil)
	persistProgress := func() {
		if !liveProgress || progressErr != nil {
			return
		}
		record.Steps = executionStepRecords(execution.Steps, prepared.RecordOutput)
		if err := r.Store.Save(record); err != nil {
			progressErr = err
			stopping = true
			stopReason = "workflow state could not be persisted"
			externalDone = nil
			cancelWorkflow()
		}
	}

	for finished < len(workflow.Steps) {
		stateChanged := false
		if !stopping && ctx.Err() != nil {
			stopping = true
			externalCanceled = true
			stopReason = "workflow canceled"
			externalDone = nil
			cancelWorkflow()
		}

		if !stopping {
			for index, step := range workflow.Steps {
				if running >= prepared.MaxParallel {
					break
				}
				if states[index] != stepPending {
					continue
				}
				ready, failedDependency := dependencyReadiness(dependencies[index], statuses)
				if failedDependency != "" {
					result := skippedStep(step, fmt.Sprintf("dependency %q did not succeed", failedDependency))
					result.AgentVersion = agentVersions[step.Agent]
					execution.Steps[index] = result
					stateChanged = true
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
				execution.Steps[index].Status = "running"
				execution.Steps[index].StartedAt = time.Now().UTC()
				stateChanged = true
				stepContext := copyTemplateContext(templateData)
				go func(index int, step Step, data templateContext) {
					result := stepRunner.runStep(
						workflowContext, prepared.StepCwds[step.ID], tempDir, step, data,
						prepared.RecordOutput, runID,
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
				result.AgentVersion = agentVersions[step.Agent]
				execution.Steps[index] = result
				stateChanged = true
				states[index] = stepFinished
				statuses[step.ID] = result.Status
				finished++
			}
		}
		if stateChanged {
			persistProgress()
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
			result.AgentVersion = agentVersions[workflow.Steps[index].Agent]
			running--
			finished++
			states[index] = stepFinished
			if stopping && result.Status == "canceled" && index != fatalIndex {
				result.Error = stopReason
			}
			execution.Steps[index] = result
			persistProgress()
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

	versionBudget := 500*time.Millisecond - time.Since(versionDetectionStarted)
	if versionBudget < 0 {
		versionBudget = 0
	}
	versionTimer := time.NewTimer(versionBudget)
	defer versionTimer.Stop()
	for remaining := len(versionChecks); remaining > 0; remaining-- {
		select {
		case detected := <-versionResults:
			if detected.version != "" {
				agentVersions[detected.agent] = detected.version
			}
		case <-versionTimer.C:
			remaining = 0
		}
	}

	for index := range execution.Steps {
		execution.Steps[index].AgentVersion = agentVersions[execution.Steps[index].Agent]
	}
	summarizeExecution(&execution, externalCanceled, fatalIndex)
	execution.Usage = summarizeUsage(execution.Steps)
	record.EndedAt = time.Now().UTC()
	record.Status = execution.Status
	for _, step := range execution.Steps {
		record.RetryNotices += step.RetryNotices
	}
	record.ParseStatus = "not_applicable"
	for _, step := range execution.Steps {
		if step.ParseStatus == "parse_error" || step.ParseStatus == "truncated" {
			record.ParseStatus = step.ParseStatus
			record.ErrorKind = step.ErrorKind
			record.Error = step.Error
			break
		}
	}
	record.ExitCode = execution.ExitCode
	record.Usage = execution.Usage
	record.Steps = executionStepRecords(execution.Steps, prepared.RecordOutput)
	if !prepared.RecordOutput {
		for index := range execution.Steps {
			execution.Steps[index].OutputFile = ""
		}
	}
	if err := r.Store.Save(record); err != nil {
		return execution, err
	}
	if progressErr != nil {
		return execution, progressErr
	}
	return execution, nil
}

func executionStepRecords(steps []StepResult, recordOutput bool) []runstore.StepRecord {
	records := make([]runstore.StepRecord, 0, len(steps))
	for _, step := range steps {
		outputFile := ""
		if recordOutput {
			outputFile = step.OutputFile
		}
		records = append(records, runstore.StepRecord{
			RetryNotices: step.RetryNotices,
			ID:           step.ID, Agent: step.Agent, AgentVersion: step.AgentVersion,
			Command: step.Command, Cwd: step.Cwd, ParseStatus: step.ParseStatus, ErrorKind: step.ErrorKind, Status: step.Status, ExitCode: step.ExitCode,
			SessionID: step.SessionID, OutputFile: outputFile, StartedAt: step.StartedAt,
			EndedAt: step.EndedAt, Attempts: step.Attempts, Error: step.Error, Usage: step.Usage,
		})
	}
	return records
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
		execution.ExitCode = result.CLIExitCode
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
		execution.ExitCode = result.CLIExitCode
		if execution.ExitCode == 0 {
			execution.ExitCode = 1
		}
		return
	}
	execution.Status = "success"
	execution.ExitCode = 0
}

func (r *Runner) runStep(ctx context.Context, workflowCwd, tempDir string, step Step, templateData templateContext, recordOutput bool, runID string) (result StepResult) {
	result = StepResult{
		ID: step.ID, Agent: step.Agent, Command: step.Command,
		Status: "failed", ExitCode: -1, ErrorKind: "start_failed", CLIExitCode: 1, StartedAt: time.Now().UTC(),
	}
	defer func() { result.EndedAt = time.Now().UTC() }()

	var err error
	args := make([]string, len(step.Args))
	for i, value := range step.Args {
		args[i], err = r.render(value, templateData)
		if err != nil {
			result.Error = err.Error()
			return result
		}
	}
	directory, err := resolveDirectory(workflowCwd, "")
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Cwd = directory
	var definition agent.Definition
	var spec agent.CommandSpec
	var environment []string
	commandStep := step.Command != ""
	if commandStep {
		commandName, resolveErr := r.render(step.Command, templateData)
		if resolveErr != nil {
			result.Error = resolveErr.Error()
			return result
		}
		result.Command = commandName
		spec = agent.CommandSpec{Command: commandName, Args: args}
		environment, err = xruntime.BuildEnvironment(os.Environ(), r.Config, config.AgentConfig{}, step.Network)
	} else {
		prompt, resolveErr := r.render(step.Prompt, templateData)
		if resolveErr != nil {
			result.Error = resolveErr.Error()
			return result
		}
		definition, err = r.Registry.Get(step.Agent)
		if err == nil {
			spec, err = definition.Run(prompt, true, args)
		}
		if err == nil {
			environment, err = xruntime.BuildEnvironment(os.Environ(), r.Config, definition.Config, step.Network)
		}
	}
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
			label := step.Agent
			if commandStep {
				label = spec.Command
			}
			fmt.Fprintf(r.Progress, "[%d/%d] %s (%s)\n", attempt, step.Retries+1, step.ID, label)
		}
		stepContext := ctx
		cancel := func() {}
		if timeout > 0 {
			stepContext, cancel = context.WithTimeout(ctx, timeout)
		}
		if r.workspaceRoot != "" {
			if err := worktree.CheckDirectory(r.workspaceRoot, directory); err != nil {
				cancel()
				result.Error = err.Error()
				return result
			}
		}
		if _, err := resolveDirectory(directory, ""); err != nil {
			cancel()
			result.Error = err.Error()
			return result
		}
		var raw *os.File
		var rawWriter io.Writer
		if recordOutput {
			raw, err = r.Store.OpenOutput(runID, fmt.Sprintf("%s.%d.raw.log", step.ID, attempt))
			if err != nil {
				cancel()
				result.Error = err.Error()
				return result
			}
			rawWriter = raw
		}
		attemptDefinition := definition
		if commandStep {
			attemptDefinition = agent.Definition{Name: step.Command, Config: config.AgentConfig{Adapter: "generic", Output: "text"}}
		}
		outcome, runErr := xruntime.Execute(stepContext, attemptDefinition, spec, xruntime.ProcessOptions{
			Dir: directory, Env: environment, Stderr: r.Stderr, RawOutput: rawWriter, SeparateProcess: true,
		}, runID, step.ID, attempt, r.Events)
		cancel()
		if raw != nil {
			if err := raw.Close(); err != nil {
				result.Error = err.Error()
				return result
			}
		}
		processResult := outcome.Process
		result.Diagnostics = string(processResult.StderrTail)
		parsed := outcome.Result
		result.Output = parsed.Output
		result.SessionID = parsed.SessionID
		result.ParseStatus = parsed.ParseStatus
		result.RetryNotices += parsed.RetryNotices
		result.ErrorKind = parsed.ErrorKind
		result.Status = parsed.Status
		result.Error = parsed.Error
		result.CLIExitCode = outcome.CLIExitCode
		if parsed.Usage != nil {
			if result.Usage == nil {
				result.Usage = &agent.Usage{}
			}
			result.Usage.Add(parsed.Usage)
		}
		if runErr != nil {
			result.ExitCode = parsed.ExitCode
			result.Error = runErr.Error()
			continue
		}

		result.ExitCode = processResult.ExitCode
		outputPath := filepath.Join(tempDir, step.ID+".output")
		if err := os.WriteFile(outputPath, []byte(result.Output), 0o600); err != nil {
			result.Status = "failed"
			result.CLIExitCode = 1
			result.Error = fmt.Sprintf("write temporary step output: %v", err)
			continue
		}
		result.OutputFile = outputPath
		if recordOutput {
			persistent, err := r.Store.SaveOutput(runID, fmt.Sprintf("%s.%d.output", step.ID, attempt), []byte(result.Output))
			if err != nil {
				result.Status = "failed"
				result.CLIExitCode = 1
				result.Error = err.Error()
				continue
			}
			result.OutputFile = persistent
		}
		if result.Status == "success" {
			return result
		}
		if result.Status == "canceled" || result.Status == "timed_out" {
			return result
		}
	}
	return result
}

func summarizeUsage(steps []StepResult) *agent.Usage {
	var result *agent.Usage
	for _, step := range steps {
		if step.Usage == nil {
			continue
		}
		if result == nil {
			result = &agent.Usage{}
		}
		result.Add(step.Usage)
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
	return StepResult{
		ID: step.ID, Agent: step.Agent, Command: step.Command,
		Status: "skipped", ExitCode: 1, Error: reason,
	}
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

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/jobs"
	"github.com/Garden12138/xcli/internal/runstore"
	xruntime "github.com/Garden12138/xcli/internal/runtime"
	"github.com/Garden12138/xcli/internal/workflow"
	"github.com/spf13/cobra"
)

const (
	jobLockFD    = 3
	jobGateFD    = 4
	jobPayloadFD = 5
)

var executablePath = os.Executable

var detachedCommand = func(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

type nonInteractiveOutcome struct {
	Process xruntime.ProcessResult
	Result  agent.RunResult
}

type synchronizedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *synchronizedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(data)
}

func executeNonInteractive(
	ctx context.Context,
	definition agent.Definition,
	name string,
	spec agent.CommandSpec,
	cwd string,
	environment []string,
	structured bool,
	stdout io.Writer,
	stderr io.Writer,
	capture bool,
	separateProcess bool,
) (nonInteractiveOutcome, error) {
	processResult, err := xruntime.RunProcess(ctx, spec, xruntime.ProcessOptions{
		Dir: cwd, Env: environment, Stdout: stdout, Stderr: stderr,
		CaptureStdout: capture, SeparateProcess: separateProcess,
	})
	if err != nil {
		return nonInteractiveOutcome{}, err
	}
	outcome := nonInteractiveOutcome{Process: processResult}
	if structured {
		result := agent.ParseStructured(definition.Config.Adapter, definition.Config.Output, processResult.Stdout)
		result.Agent = name
		result.ExitCode = processResult.ExitCode
		result.Status = processStatus(processResult)
		outcome.Result = result
	}
	return outcome, nil
}

type detachedRunOptions struct {
	ConfigPath   string
	Agent        string
	Prompt       string
	NativeArgs   []string
	Cwd          string
	Structured   bool
	RecordOutput bool
	Record       runstore.Record
}

type detachedRunPayload struct {
	Agent        string   `json:"agent"`
	Prompt       string   `json:"prompt"`
	NativeArgs   []string `json:"native_args,omitempty"`
	Cwd          string   `json:"cwd"`
	Structured   bool     `json:"structured,omitempty"`
	RecordOutput bool     `json:"record_output,omitempty"`
}

type detachedJobPayload struct {
	Kind     string                `json:"kind"`
	Run      *detachedRunPayload   `json:"run,omitempty"`
	Workflow *workflow.PreparedRun `json:"workflow,omitempty"`
}

func startDetachedRun(store *runstore.Store, options detachedRunOptions) (jobs.View, error) {
	payload := detachedJobPayload{Kind: "run", Run: &detachedRunPayload{
		Agent: options.Agent, Prompt: options.Prompt, NativeArgs: options.NativeArgs,
		Cwd: options.Cwd, Structured: options.Structured, RecordOutput: options.RecordOutput,
	}}
	return startDetachedJob(store, options.ConfigPath, options.Record, payload)
}

func startDetachedWorkflow(store *runstore.Store, configPath string, record runstore.Record, prepared workflow.PreparedRun) (jobs.View, error) {
	return startDetachedJob(store, configPath, record, detachedJobPayload{Kind: "workflow", Workflow: &prepared})
}

func startDetachedJob(store *runstore.Store, configPath string, record runstore.Record, payload detachedJobPayload) (jobs.View, error) {
	manager := jobs.Manager{Store: store}
	files, err := manager.CreateFiles(record.ID)
	if err != nil {
		return jobs.View{}, err
	}
	defer files.Log.Close()
	defer files.Lock.Close()

	gateRead, gateWrite, err := os.Pipe()
	if err != nil {
		return jobs.View{}, fmt.Errorf("create job startup gate: %w", err)
	}
	defer gateRead.Close()
	defer gateWrite.Close()
	payloadRead, payloadWrite, err := os.Pipe()
	if err != nil {
		return jobs.View{}, fmt.Errorf("create job payload pipe: %w", err)
	}
	defer payloadRead.Close()
	defer payloadWrite.Close()

	executable, err := executablePath()
	if err != nil {
		return jobs.View{}, fmt.Errorf("resolve xcli executable: %w", err)
	}
	args := []string{"--config", configPath, "__job-worker", record.ID}
	worker := detachedCommand(executable, args...)
	jobs.ConfigureDetached(worker)
	worker.ExtraFiles = []*os.File{files.Lock, gateRead, payloadRead}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return jobs.View{}, err
	}
	defer devNull.Close()
	worker.Stdin = devNull
	worker.Stdout = devNull
	worker.Stderr = devNull

	record.Background = true
	record.LogFile = files.LogPath
	record.Status = "running"
	if err := worker.Start(); err != nil {
		record.Status = "failed"
		record.ExitCode = 1
		record.EndedAt = time.Now().UTC()
		_ = store.Save(record)
		return jobs.View{}, fmt.Errorf("start background worker: %w", err)
	}
	record.PID = worker.Process.Pid
	_ = gateRead.Close()
	_ = payloadRead.Close()
	if err := store.Save(record); err != nil {
		_ = jobs.Terminate(worker.Process.Pid, true)
		return jobs.View{}, err
	}
	if err := json.NewEncoder(payloadWrite).Encode(payload); err != nil {
		_ = jobs.Terminate(worker.Process.Pid, true)
		markDetachedStartFailed(store, &record)
		return jobs.View{}, fmt.Errorf("send background job payload: %w", err)
	}
	if err := payloadWrite.Close(); err != nil {
		_ = jobs.Terminate(worker.Process.Pid, true)
		markDetachedStartFailed(store, &record)
		return jobs.View{}, fmt.Errorf("close background job payload: %w", err)
	}
	if err := gateWrite.Close(); err != nil {
		_ = jobs.Terminate(worker.Process.Pid, true)
		markDetachedStartFailed(store, &record)
		return jobs.View{}, fmt.Errorf("release background worker: %w", err)
	}
	_ = worker.Process.Release()
	return jobs.ToView(record), nil
}

func markDetachedStartFailed(store *runstore.Store, record *runstore.Record) {
	updated, err := (jobs.Manager{Store: store}).MarkFailed(*record, "background worker could not be started")
	if err == nil {
		*record = updated
	}
}

func (a *app) newJobWorkerCommand() *cobra.Command {
	command := &cobra.Command{
		Use:    "__job-worker <id>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return a.runJobWorker(command, args[0])
		},
	}
	return command
}

func (a *app) runJobWorker(command *cobra.Command, id string) error {
	ctx, cancel := signalContext(command.Context())
	defer cancel()
	lockFile := os.NewFile(jobLockFD, "job-lock")
	gateFile := os.NewFile(jobGateFD, "job-gate")
	payloadFile := os.NewFile(jobPayloadFD, "job-payload")
	if lockFile == nil || gateFile == nil || payloadFile == nil {
		return errors.New("background worker file descriptors are unavailable")
	}
	defer lockFile.Close()
	var payload detachedJobPayload
	decoder := json.NewDecoder(payloadFile)
	if err := decoder.Decode(&payload); err != nil {
		payloadFile.Close()
		return fmt.Errorf("decode background job payload: %w", err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		payloadFile.Close()
		if err == nil {
			return errors.New("decode background job payload: multiple JSON values")
		}
		return fmt.Errorf("decode background job payload: %w", err)
	}
	payloadFile.Close()
	if _, err := io.Copy(io.Discard, gateFile); err != nil {
		gateFile.Close()
		return fmt.Errorf("wait for job startup gate: %w", err)
	}
	gateFile.Close()

	store, err := newStore()
	if err != nil {
		return err
	}
	record, err := store.Load(id)
	if err != nil {
		return err
	}
	manager := jobs.Manager{Store: store}
	logPath, err := manager.LogPath(id)
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	logWriter := &synchronizedWriter{writer: logFile}
	fail := func(runErr error) error {
		fmt.Fprintln(logWriter, "Error:", runErr)
		if latest, loadErr := store.Load(id); loadErr == nil {
			record = latest
		}
		if _, saveErr := manager.MarkFailed(record, runErr.Error()); saveErr != nil {
			return saveErr
		}
		return runErr
	}

	cfg, _, registry, err := a.load()
	if err != nil {
		return fail(err)
	}

	switch payload.Kind {
	case "run":
		if payload.Run == nil {
			return fail(errors.New("background run payload is missing"))
		}
		return a.runDetachedRunWorker(ctx, cfg, registry, store, &record, logWriter, *payload.Run, fail)
	case "workflow":
		if payload.Workflow == nil {
			return fail(errors.New("background workflow payload is missing"))
		}
		runner := workflow.Runner{
			Config: cfg, Registry: registry, Store: store, Progress: logWriter, Stderr: logWriter,
		}
		execution, runErr := runner.RunPrepared(ctx, *payload.Workflow, &record)
		if runErr != nil {
			return fail(runErr)
		}
		if err := writeWorkflowExecution(logWriter, execution); err != nil {
			return fail(err)
		}
		if execution.ExitCode != 0 {
			return &ExitError{Code: execution.ExitCode}
		}
		return nil
	default:
		return fail(fmt.Errorf("unsupported background job kind %q", payload.Kind))
	}
}

func (a *app) runDetachedRunWorker(
	ctx context.Context,
	cfg config.Config,
	registry *agent.Registry,
	store *runstore.Store,
	record *runstore.Record,
	logWriter io.Writer,
	payload detachedRunPayload,
	fail func(error) error,
) error {
	definition, err := registry.Get(payload.Agent)
	if err != nil {
		return fail(err)
	}
	spec, err := definition.Run(payload.Prompt, payload.Structured, payload.NativeArgs)
	if err != nil {
		return fail(err)
	}
	environment, err := xruntime.BuildEnvironment(os.Environ(), cfg, definition.Config, "")
	if err != nil {
		return fail(err)
	}

	var stdout io.Writer = logWriter
	if payload.Structured {
		stdout = nil
	}
	outcome, err := executeNonInteractive(
		ctx, definition, payload.Agent, spec, payload.Cwd, environment, payload.Structured,
		stdout, logWriter, payload.Structured || payload.RecordOutput, false,
	)
	record.EndedAt = time.Now().UTC()
	if err != nil {
		return fail(err)
	}
	if ctx.Err() != nil && !outcome.Process.Canceled {
		outcome.Process.Canceled = true
		outcome.Process.ExitCode = 130
	}
	record.ExitCode = outcome.Process.ExitCode
	record.Status = processStatus(outcome.Process)
	if payload.Structured {
		record.SessionID = outcome.Result.SessionID
		record.Usage = outcome.Result.Usage
		if outcome.Result.Output != "" {
			if _, err := fmt.Fprintln(logWriter, outcome.Result.Output); err != nil {
				return fail(err)
			}
		}
	}
	if payload.RecordOutput {
		path, err := store.SaveOutput(record.ID, "output.log", outcome.Process.Stdout)
		if err != nil {
			return fail(err)
		}
		record.OutputFile = path
	}
	if err := store.Save(*record); err != nil {
		return err
	}
	if outcome.Process.ExitCode != 0 {
		return &ExitError{Code: outcome.Process.ExitCode}
	}
	return nil
}

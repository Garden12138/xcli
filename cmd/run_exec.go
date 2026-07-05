package cmd

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/jobs"
	"github.com/Garden12138/xcli/internal/runstore"
	xruntime "github.com/Garden12138/xcli/internal/runtime"
	"github.com/spf13/cobra"
)

const (
	jobLockFD = 3
	jobGateFD = 4
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

func startDetachedRun(store *runstore.Store, options detachedRunOptions) (jobs.View, error) {
	manager := jobs.Manager{Store: store}
	files, err := manager.CreateFiles(options.Record.ID)
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

	executable, err := executablePath()
	if err != nil {
		return jobs.View{}, fmt.Errorf("resolve xcli executable: %w", err)
	}
	prompt := base64.RawStdEncoding.EncodeToString([]byte(options.Prompt))
	args := []string{
		"--config", options.ConfigPath, "__run-job",
		"--cwd", options.Cwd,
	}
	if options.Structured {
		args = append(args, "--structured")
	}
	if options.RecordOutput {
		args = append(args, "--record-output")
	}
	args = append(args, options.Record.ID, options.Agent, prompt)
	if len(options.NativeArgs) > 0 {
		args = append(args, "--")
		args = append(args, options.NativeArgs...)
	}
	worker := detachedCommand(executable, args...)
	jobs.ConfigureDetached(worker)
	worker.ExtraFiles = []*os.File{files.Lock, gateRead}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return jobs.View{}, err
	}
	defer devNull.Close()
	worker.Stdin = devNull
	worker.Stdout = devNull
	worker.Stderr = devNull

	record := options.Record
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
	if err := store.Save(record); err != nil {
		_ = jobs.Terminate(worker.Process.Pid, true)
		return jobs.View{}, err
	}
	if err := gateWrite.Close(); err != nil {
		_ = jobs.Terminate(worker.Process.Pid, true)
		return jobs.View{}, fmt.Errorf("release background worker: %w", err)
	}
	_ = worker.Process.Release()
	return jobs.ToView(record), nil
}

func (a *app) newJobWorkerCommand() *cobra.Command {
	var cwd string
	var structured bool
	var recordOutput bool
	command := &cobra.Command{
		Use:    "__run-job <id> <agent> <prompt-base64> [-- native-args...]",
		Hidden: true,
		Args: func(command *cobra.Command, args []string) error {
			before, _ := splitNativeArgs(command, args)
			if len(before) != 3 {
				return errors.New("invalid background worker arguments")
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			before, nativeArgs := splitNativeArgs(command, args)
			promptBytes, err := base64.RawStdEncoding.DecodeString(before[2])
			if err != nil {
				return fmt.Errorf("decode job prompt: %w", err)
			}
			return a.runJobWorker(command, before[0], before[1], string(promptBytes), cwd, structured, recordOutput, nativeArgs)
		},
	}
	command.Flags().StringVar(&cwd, "cwd", "", "worker cwd")
	command.Flags().BoolVar(&structured, "structured", false, "capture structured output")
	command.Flags().BoolVar(&recordOutput, "record-output", false, "persist raw output")
	return command
}

func (a *app) runJobWorker(
	command *cobra.Command,
	id string,
	name string,
	prompt string,
	cwd string,
	structured bool,
	recordOutput bool,
	nativeArgs []string,
) error {
	lockFile := os.NewFile(jobLockFD, "job-lock")
	gateFile := os.NewFile(jobGateFD, "job-gate")
	if lockFile == nil || gateFile == nil {
		return errors.New("background worker file descriptors are unavailable")
	}
	defer lockFile.Close()
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
		record.Status = "failed"
		record.ExitCode = 1
		record.EndedAt = time.Now().UTC()
		if saveErr := store.Save(record); saveErr != nil {
			return saveErr
		}
		return runErr
	}

	cfg, _, registry, err := a.load()
	if err != nil {
		return fail(err)
	}
	definition, err := registry.Get(name)
	if err != nil {
		return fail(err)
	}
	spec, err := definition.Run(prompt, structured, nativeArgs)
	if err != nil {
		return fail(err)
	}
	environment, err := xruntime.BuildEnvironment(os.Environ(), cfg, definition.Config, "")
	if err != nil {
		return fail(err)
	}

	ctx, cancel := signalContext(command.Context())
	defer cancel()
	var stdout io.Writer = logWriter
	if structured {
		stdout = nil
	}
	outcome, err := executeNonInteractive(
		ctx, definition, name, spec, cwd, environment, structured,
		stdout, logWriter, structured || recordOutput, false,
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
	if structured {
		record.SessionID = outcome.Result.SessionID
		record.Usage = outcome.Result.Usage
		if outcome.Result.Output != "" {
			if _, err := fmt.Fprintln(logWriter, outcome.Result.Output); err != nil {
				return fail(err)
			}
		}
	}
	if recordOutput {
		path, err := store.SaveOutput(record.ID, "output.log", outcome.Process.Stdout)
		if err != nil {
			return fail(err)
		}
		record.OutputFile = path
	}
	if err := store.Save(record); err != nil {
		return err
	}
	if outcome.Process.ExitCode != 0 {
		return &ExitError{Code: outcome.Process.ExitCode}
	}
	return nil
}

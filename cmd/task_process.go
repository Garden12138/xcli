package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/jobs"
	"github.com/Garden12138/xcli/internal/task"
	"github.com/Garden12138/xcli/internal/workflow"
	"github.com/spf13/cobra"
)

type taskWorkerPayload struct {
	Config config.Config
	Update task.Update
}

func startDetachedTask(r *task.Runner, id string, u task.Update) (task.Task, error) {
	lock, err := r.Store.Lock(id, "owner.lock", false)
	if errors.Is(err, task.ErrOwned) {
		return r.Continue(context.Background(), id, u)
	}
	if err != nil {
		return task.Task{}, err
	}
	defer lock.Close()
	t, err := r.Store.Load(id)
	if err != nil {
		return t, err
	}
	c, err := r.Store.Control(id)
	if err != nil {
		return t, err
	}
	t.StopBaseline = c.StopSequence
	// Transfer the already-held lease to the child; no startup ownership gap.
	input, output, err := os.Pipe()
	if err != nil {
		return t, err
	}
	defer input.Close()
	defer output.Close()
	executable, err := executablePath()
	if err != nil {
		return t, err
	}
	worker := detachedCommand(executable, "__task-worker", id)
	jobs.ConfigureDetached(worker)
	worker.ExtraFiles = []*os.File{lock, input}
	log, err := os.OpenFile(filepath.Join(r.Store.Dir(id), "worker.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return t, err
	}
	defer log.Close()
	worker.Stderr = log
	if err := worker.Start(); err != nil {
		return t, err
	}
	input.Close()
	t.Background, t.PID, t.OwnerActive = true, worker.Process.Pid, true
	if t.Checkpoint != nil && t.Checkpoint.Stage != "done" {
		t.Status, t.ExitCode, t.Reason, t.EndedAt = "running", 0, "Background execution started; inspect Task status for the result.", time.Time{}
	}
	if err := r.Store.Save(&t); err != nil {
		output.Close()
		_ = jobs.Terminate(worker.Process.Pid, true)
		_ = worker.Wait()
		return t, err
	}
	if err := json.NewEncoder(output).Encode(taskWorkerPayload{Config: r.Config, Update: u}); err != nil {
		output.Close()
		_ = worker.Wait()
		return t, err
	}
	output.Close()
	_ = worker.Process.Release()
	return t, nil
}

func (a *app) newTaskWorkerCommand() *cobra.Command {
	return &cobra.Command{Use: "__task-worker <id>", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		lock, input := os.NewFile(3, "task-owner"), os.NewFile(4, "task-input")
		if lock == nil || input == nil {
			return errors.New("Task worker descriptors missing")
		}
		defer lock.Close()
		defer input.Close()
		syscall.CloseOnExec(int(lock.Fd()))
		syscall.CloseOnExec(int(input.Fd()))
		var payload taskWorkerPayload
		if err := json.NewDecoder(io.LimitReader(input, 4<<20)).Decode(&payload); err != nil {
			return err
		}
		store, err := task.NewStore()
		if err != nil {
			return err
		}
		runs, err := newStore()
		if err != nil {
			return err
		}
		r := &task.Runner{Config: payload.Config, Registry: agent.NewRegistry(payload.Config), Store: store, Runs: runs, Lease: lock}
		r.Execute = supervisedAttempt(r)
		ctx, cancel := signalContext(command.Context())
		defer cancel()
		t, err := r.Continue(ctx, args[0], payload.Update)
		if err != nil {
			return err
		}
		if t.ExitCode != 0 {
			return &ExitError{Code: t.ExitCode, Message: t.Reason}
		}
		return nil
	}}
}

type taskAttemptPayload struct {
	Config       config.Config
	TaskID       string
	RunID        string
	Cwd          string
	Boundary     string
	Step         workflow.Step
	RecordOutput bool
}
type taskAttemptMessage struct {
	Event   *agent.Event  `json:"event,omitempty"`
	Receipt *task.Receipt `json:"receipt,omitempty"`
}

// The supervisor owns the native child and watches a pipe owned only by the
// Task controller. EOF (including SIGKILL of that controller) cancels the child.
func supervisedAttempt(r *task.Runner) func(context.Context, string, string, string, workflow.Step, bool, func(agent.Event) error) workflow.StepResult {
	return func(ctx context.Context, id, cwd, boundary string, step workflow.Step, raw bool, sink func(agent.Event) error) workflow.StepResult {
		fail := func(err error) workflow.StepResult {
			return workflow.StepResult{ID: step.ID, Status: "failed", ExitCode: 1, CLIExitCode: 1, ErrorKind: "supervisor_failed", Error: err.Error()}
		}
		// Obtain Task ID from the Run rather than mutable Runner internals.
		record, err := r.Runs.Load(id)
		if err != nil {
			return fail(err)
		}
		lease, err := r.Store.Lock(record.TaskID, "attempt.lock", false)
		if err != nil {
			return fail(err)
		}
		defer lease.Close()
		liveR, liveW, err := os.Pipe()
		if err != nil {
			return fail(err)
		}
		defer liveR.Close()
		defer liveW.Close()
		dataR, dataW, err := os.Pipe()
		if err != nil {
			return fail(err)
		}
		defer dataR.Close()
		defer dataW.Close()
		executable, err := executablePath()
		if err != nil {
			return fail(err)
		}
		child := detachedCommand(executable, "__task-attempt")
		jobs.ConfigureDetached(child)
		child.ExtraFiles = []*os.File{lease, liveR, dataR}
		child.Stderr = r.Stderr
		stream, err := child.StdoutPipe()
		if err != nil {
			return fail(err)
		}
		if err := child.Start(); err != nil {
			return fail(err)
		}
		liveR.Close()
		dataR.Close()
		closed := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				liveW.Close()
			case <-closed:
			}
		}()
		defer close(closed)
		if err := json.NewEncoder(dataW).Encode(taskAttemptPayload{Config: r.Config, TaskID: record.TaskID, RunID: id, Cwd: cwd, Boundary: boundary, Step: step, RecordOutput: raw}); err != nil {
			dataW.Close()
			liveW.Close()
			_ = child.Wait()
			return fail(err)
		}
		dataW.Close()
		decoder := json.NewDecoder(stream)
		var result *task.Receipt
		var deliveryErr error
		for {
			var msg taskAttemptMessage
			err := decoder.Decode(&msg)
			if err == io.EOF {
				break
			}
			if err != nil {
				deliveryErr = err
				liveW.Close()
				break
			}
			if msg.Event != nil && sink != nil && deliveryErr == nil {
				if err := sink(*msg.Event); err != nil {
					deliveryErr = err
					liveW.Close()
				}
			}
			if msg.Receipt != nil {
				result = msg.Receipt
			}
		}
		waitErr := child.Wait()
		if deliveryErr != nil {
			return fail(deliveryErr)
		}
		if waitErr != nil {
			return fail(waitErr)
		}
		if result == nil {
			return fail(errors.New("Task supervisor exited without receipt"))
		}
		result.Result.CLIExitCode = result.CLIExitCode
		result.Result.Diagnostics = result.Diagnostics
		return result.Result
	}
}

func (a *app) newTaskAttemptCommand() *cobra.Command {
	return &cobra.Command{Use: "__task-attempt", Hidden: true, Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		lease, live, input := os.NewFile(3, "attempt-lock"), os.NewFile(4, "owner-life"), os.NewFile(5, "attempt-payload")
		if lease == nil || live == nil || input == nil {
			return errors.New("Task attempt descriptors missing")
		}
		defer lease.Close()
		defer live.Close()
		defer input.Close()
		syscall.CloseOnExec(int(lease.Fd()))
		syscall.CloseOnExec(int(live.Fd()))
		syscall.CloseOnExec(int(input.Fd()))
		var payload taskAttemptPayload
		if err := json.NewDecoder(io.LimitReader(input, 4<<20)).Decode(&payload); err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(command.Context())
		defer cancel()
		go func() { var b [1]byte; _, _ = live.Read(b[:]); cancel() }()
		runs, err := newStore()
		if err != nil {
			return err
		}
		store, err := task.NewStore()
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(command.OutOrStdout())
		var mu sync.Mutex
		wr := workflow.Runner{Config: payload.Config, Registry: agent.NewRegistry(payload.Config), Store: runs, Events: func(e agent.Event) error {
			mu.Lock()
			defer mu.Unlock()
			return encoder.Encode(taskAttemptMessage{Event: &e})
		}}
		res := wr.ExecuteAttempt(ctx, payload.RunID, payload.Cwd, payload.Boundary, payload.Step, payload.RecordOutput)
		t, err := store.Load(payload.TaskID)
		if err != nil {
			return err
		}
		root := t.Output
		if t.Workspace != nil {
			root = t.Workspace.Path
		}
		hash, err := task.MaterialSnapshot(root, t.Kind == "project")
		if err != nil {
			return err
		}
		receipt := task.Receipt{Result: res, CLIExitCode: res.CLIExitCode, Snapshot: hash, Diagnostics: res.Diagnostics}
		data, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		if err := store.Write(payload.TaskID, payload.RunID+".receipt.json", data); err != nil {
			return fmt.Errorf("save attempt receipt: %w", err)
		}
		mu.Lock()
		defer mu.Unlock()
		return encoder.Encode(taskAttemptMessage{Receipt: &receipt})
	}}
}

func waitTaskStop(ctx context.Context, store *task.Store, id string, timeout time.Duration) (task.Task, error) {
	if timeout <= 0 {
		return task.Task{}, errors.New("stop timeout must be positive")
	}
	if _, err := store.Queue(id, task.Update{}, true); err != nil {
		return task.Task{}, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		t, err := store.Inspect(id)
		if err != nil {
			return t, err
		}
		attemptActive, err := store.Active(id, "attempt.lock")
		if err != nil {
			return t, err
		}
		if !t.OwnerActive && !attemptActive {
			return t, nil
		}
		select {
		case <-ctx.Done():
			return t, ctx.Err()
		case <-timer.C:
			return t, errors.New("Task stop is still pending; inspect status before continuing")
		case <-ticker.C:
		}
	}
}

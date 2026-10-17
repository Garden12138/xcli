package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/runstore"
	"github.com/Garden12138/xcli/internal/workflow"
	"github.com/Garden12138/xcli/internal/worktree"
)

func (r *Runner) save() error {
	if !r.activeStart.IsZero() {
		now := time.Now()
		r.task.Elapsed += now.Sub(r.activeStart)
		r.activeStart = now
		r.task.ActiveAt = now.UTC()
	}
	return r.Store.Save(r.task)
}
func (r *Runner) commit() error { r.task.Checkpoint.InFlight = ""; return r.save() }
func (r *Runner) binding(d agent.Definition) string {
	entry := map[string]string{}
	for _, name := range []string{"HOME", "CODEX_HOME", "CURSOR_CONFIG_DIR", "XDG_CONFIG_HOME"} {
		entry[name] = os.Getenv(name)
	}
	data, _ := json.Marshal(struct {
		Agent   any
		Network any
		Entry   any
	}{d.Config, r.Config.Networks[d.Config.Network], entry})
	return digest(data)
}

func (r *Runner) Create(ctx context.Context, opts Options) (Task, error) {
	if strings.TrimSpace(opts.Goal) == "" {
		return Task{}, errors.New("task goal is required")
	}
	if opts.Timeout <= 0 || opts.MaxCalls <= 0 {
		return Task{}, errors.New("timeout and max-calls must be positive")
	}
	now := time.Now().UTC()
	t := &Task{Version: 2, ID: runstore.NewID("task"), Goal: opts.Goal, OriginalGoal: opts.Goal, Kind: "work", Mode: "auto", Agent: opts.Agent, Status: "planning", StartedAt: now, MaxCalls: opts.MaxCalls, Options: opts, Candidates: []string{}, Bindings: map[string]string{}}
	if opts.Agent != "" {
		t.Mode = "single"
	}
	lock, err := r.Store.Lock(t.ID, "owner.lock", false)
	if err != nil {
		return Task{}, err
	}
	defer lock.Close()
	r.task, r.options, r.activeStart = t, opts, time.Now()
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	err = r.prepare(ctx)
	for _, name := range t.Candidates {
		d, _ := r.Registry.Get(name)
		t.Bindings[name] = r.binding(d)
	}
	if err != nil {
		t.Status, t.ExitCode, t.Reason = "failed", 1, err.Error()
	} else {
		t.Checkpoint = &Checkpoint{Stage: "planning"}
	}
	saveErr := r.save()
	r.activeStart = time.Time{}
	t.ActiveAt = time.Time{}
	if saveErr == nil {
		saveErr = r.Store.Save(t)
	}
	if saveErr != nil {
		return *t, saveErr
	}
	return *t, err
}

func hasUpdate(u Update) bool {
	return u.Message != "" || len(u.Verify) > 0 || u.Timeout != 0 || u.MaxCalls != 0 || u.Reconcile || u.FreshSession
}

func (r *Runner) Continue(ctx context.Context, id string, u Update) (Task, error) {
	lock := r.Lease
	if lock == nil {
		var err error
		lock, err = r.Store.Lock(id, "owner.lock", false)
		if errors.Is(err, ErrOwned) {
			if u.Reconcile || u.FreshSession {
				return Task{}, errors.New("recovery flags require an idle Task; stop it first")
			}
			if hasUpdate(u) {
				if _, err := r.Store.Queue(id, u, false); err != nil {
					return Task{}, err
				}
			}
			return r.Store.Inspect(id)
		}
		if err != nil {
			return Task{}, err
		}
		defer lock.Close()
	}
	t, err := r.Store.Load(id)
	if err != nil {
		return Task{}, err
	}
	if err := r.Store.Recover(&t); err != nil {
		return t, err
	}
	active, err := r.Store.Active(id, "attempt.lock")
	if err != nil {
		return t, err
	}
	if active {
		return t, errors.New("previous Task attempt is still exiting; retry after it has stopped")
	}
	r.task, r.options = &t, t.Options
	if r.Lease == nil {
		c, err := r.Store.Control(id)
		if err != nil {
			return t, err
		}
		t.StopBaseline = c.StopSequence
	}
	if err := r.recoverRuns(); err != nil {
		return t, err
	}
	r.cached, r.cachedAttempt = nil, nil
	if t.Checkpoint != nil && t.Checkpoint.InFlight != "" {
		data, err := os.ReadFile(filepath.Join(r.Store.Dir(id), t.Checkpoint.InFlight+".receipt.json"))
		if err == nil {
			var receipt Receipt
			if err := json.Unmarshal(data, &receipt); err != nil {
				return t, err
			}
			receipt.Result.Diagnostics = receipt.Diagnostics
			if receipt.Result.Status == "success" && receipt.CLIExitCode == 0 && receipt.Snapshot != "" {
				for i := range t.Attempts {
					if t.Attempts[i].RunID == t.Checkpoint.InFlight {
						r.cached, r.cachedAttempt = &receipt, &t.Attempts[i]
						break
					}
				}
			}
		} else if !os.IsNotExist(err) {
			return t, err
		}
	}
	if hasUpdate(u) {
		if _, err := r.Store.Queue(id, u, false); err != nil {
			return t, err
		}
	}
	if t.Checkpoint == nil || t.Version == 1 {
		if !u.Reconcile {
			return t, errors.New("legacy Task has no resumable checkpoint; inspect materials and use --reconcile --fresh-session")
		}
		if t.Cwd == "" || len(t.Candidates) == 0 {
			return t, errors.New("Task was not prepared; create a new Task")
		}
		t.Version = 2
		t.Options = Options{Goal: t.Goal, Agent: t.Agent, Timeout: t.Deadline.Sub(t.StartedAt), MaxCalls: t.MaxCalls}
		if t.Options.Timeout <= 0 {
			t.Options.Timeout = 10 * time.Minute
		}
		t.Checkpoint = &Checkpoint{Stage: "planning"}
		t.OriginalGoal = t.Goal
		t.Bindings = map[string]string{}
		for _, name := range t.Candidates {
			d, err := r.Registry.Get(name)
			if err != nil {
				return t, err
			}
			t.Bindings[name] = r.binding(d)
		}
		r.options = t.Options
	}
	r.resume, r.fresh = true, u.FreshSession
	if err := r.checkWorkspace(); err != nil {
		return t, err
	}
	for _, name := range t.Candidates {
		d, err := r.Registry.Get(name)
		if err != nil {
			return t, err
		}
		if t.Bindings[name] != r.binding(d) {
			return t, fmt.Errorf("agent configuration changed for %s; restore the original execution/login entry before continuing", name)
		}
	}
	// Unexpected input changes require acknowledgement and invalidate evidence.
	changed := false
	if err := r.checkInputs(); err != nil {
		changed = true
	}
	old := t.Snapshot
	if _, err := r.capture(""); err != nil {
		return t, err
	}
	materialChanged := old != "" && old != t.Snapshot
	if r.cached != nil && r.cached.Snapshot == t.Snapshot {
		materialChanged = false
	} else {
		r.cached, r.cachedAttempt = nil, nil
	}
	changed = changed || materialChanged
	uncertain := (t.Checkpoint.InFlight != "" || t.Status == "needs_reconcile") && r.cached == nil
	if changed {
		t.Checkpoint.Stage, t.Checkpoint.VerifyIndex, t.Checkpoint.Passed = "planning", 0, false
	}
	if (changed || uncertain) && !u.Reconcile {
		t.Status, t.ExitCode, t.Reason = "needs_reconcile", 1, "Interrupted operation or changed materials require review; use task continue --reconcile to accept current materials."
		if err := r.save(); err != nil {
			return t, err
		}
		return t, errors.New(t.Reason)
	}
	if changed {
		for i := range t.Inputs {
			data, err := boundedRead(t.Inputs[i].Path)
			if err != nil {
				return t, err
			}
			t.Inputs[i].SHA256 = digest(data)
		}
		t.Checkpoint.Stage, t.Checkpoint.VerifyIndex, t.Checkpoint.Passed = "planning", 0, false
	}
	if uncertain {
		t.Checkpoint.InFlight = ""
		r.fresh = r.fresh || changed
	}
	if t.Status == "waiting_user" && len(u.Verify) > 0 && t.Checkpoint.Stage == "decide" {
		t.Checkpoint.Stage = "accept"
	}
	if err := r.applyUpdates(); err != nil {
		return t, err
	}
	if t.Status == "completed" && t.Checkpoint.Stage == "done" {
		return t, nil
	}

	return r.runOwned(ctx)
}

func (r *Runner) checkWorkspace() error {
	t := r.task
	for _, path := range []string{t.Cwd, t.Output} {
		resolved, err := canonical(path)
		if err != nil {
			return err
		}
		if resolved != path {
			return errors.New("Task path identity changed")
		}
	}
	if t.Workspace != nil {
		if err := worktree.CheckDirectory(t.Workspace.Path, t.Cwd); err != nil {
			return err
		}
		if !inside(t.Workspace.Path, t.Output) {
			return errors.New("output escapes retained worktree")
		}
		gitdir, err := git(t.Workspace.Path, "rev-parse", "--show-toplevel")
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(gitdir)) != t.Workspace.Path {
			return errors.New("retained worktree identity changed")
		}
		head, err := git(t.Workspace.Path, "rev-parse", "HEAD")
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(head)) != t.Workspace.BaseCommit {
			return errors.New("retained worktree baseline changed")
		}
	}
	return r.checkOriginal()
}

func (r *Runner) applyUpdates() error {
	c, err := r.Store.Control(r.task.ID)
	if err != nil {
		return err
	}
	t := r.task
	for _, u := range c.Updates {
		if u.Sequence <= t.Checkpoint.Applied {
			continue
		}
		if u.Timeout != 0 && (u.Timeout < t.Options.Timeout || u.Timeout <= t.Elapsed) {
			return errors.New("--timeout must increase the total execution budget beyond time already consumed")
		}
		if u.MaxCalls != 0 && (u.MaxCalls < t.MaxCalls || u.MaxCalls < t.Calls) {
			return errors.New("--max-calls cannot reduce the cumulative budget")
		}
		if u.Timeout > 0 {
			t.Options.Timeout = u.Timeout
		}
		if u.MaxCalls > 0 {
			t.Options.MaxCalls, t.MaxCalls = u.MaxCalls, u.MaxCalls
		}
		if len(u.Verify) > 0 {
			t.Options.Verify = u.Verify
			if t.Checkpoint.Stage != "planning" && t.Checkpoint.Stage != "worker" {
				t.Checkpoint.Stage = "accept"
			}
		}
		if u.Message != "" {
			r.cached, r.cachedAttempt = nil, nil
			t.Goal += "\n\nUser update: " + u.Message
			t.Checkpoint.Stage = "planning"
			t.Status = "planning"
		}
		if u.FreshSession {
			r.fresh = true
		}
		t.Pending = nil
		t.Updates = append(t.Updates, u)
		t.Checkpoint.Applied = u.Sequence
		r.options = t.Options
		if err := r.commit(); err != nil {
			return err
		}
		if err := r.emit("input_applied", "", "", u.Message, u); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) runOwned(parent context.Context) (result Task, runErr error) {
	t := r.task
	remaining := t.Options.Timeout - t.Elapsed
	if remaining <= 0 {
		t.Status, t.ExitCode, t.Reason = "failed", 124, "task execution time budget exhausted; increase --timeout to continue"
		return *t, r.save()
	}
	ctx, stopCancel := context.WithCancel(parent)
	defer stopCancel()
	stopRequested := make(chan bool, 1)
	monitorError := make(chan error, 1)
	t.OwnerToken = runstore.NewID("owner")
	stopBaseline := t.StopBaseline
	token, taskID, baseline, clockStart := t.OwnerToken, t.ID, t.Elapsed, time.Now()
	monitorDone := make(chan struct{})
	monitorStore := *r.Store
	done := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				c, err := monitorStore.Control(taskID)
				if err == nil {
					data, marshalErr := json.Marshal(heartbeat{Owner: token, Elapsed: baseline + time.Since(clockStart)})
					if marshalErr == nil {
						err = monitorStore.Write(taskID, "heartbeat.json", data)
					} else {
						err = marshalErr
					}
				}
				if err != nil || c.StopSequence > stopBaseline {
					if err != nil {
						monitorError <- err
					}
					select {
					case stopRequested <- c.StopSequence > stopBaseline:
					default:
					}
					stopCancel()
					return
				}
			}
		}
	}()
	r.activeStart = time.Now()
	t.ActiveAt, t.PID, t.OwnerActive = time.Now().UTC(), os.Getpid(), true
	t.Status, t.ExitCode, t.Reason, t.EndedAt = "running", 0, "", time.Time{}
	t.Deadline = time.Now().UTC().Add(remaining)
	defer func() {
		close(done)
		<-monitorDone
		requested := false
		select {
		case requested = <-stopRequested:
		default:
		}
		if runErr != nil {
			t.Status, t.ExitCode, t.Reason = "failed", 1, runErr.Error()
			if ctx.Err() != nil {
				t.Status, t.ExitCode = "canceled", 130
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(runErr, context.DeadlineExceeded) {
				t.Status, t.ExitCode = "failed", 124
			}
			if requested {
				t.Status, t.ExitCode, t.Reason = "paused", 0, "Stopped; partial artifacts and checkpoint retained."
				runErr = nil
			}
		}
		select {
		case err := <-monitorError:
			t.Status, t.ExitCode, t.Reason, runErr = "failed", 1, err.Error(), err
		default:
		}
		// A controlled interruption has observed the child exit. Retrying the
		// unfinished node uses its native session or a fresh material brief.
		if t.Status == "paused" || t.Status == "canceled" {
			t.Checkpoint.InFlight = ""
			_, captureErr := r.capture("")
			if captureErr != nil {
				t.Status, t.ExitCode, t.Reason = "needs_reconcile", 1, captureErr.Error()
				runErr = captureErr
			}
		}
		t.EndedAt, t.OwnerActive = time.Now().UTC(), false
		if err := r.save(); err != nil {
			runErr = err
			t.Status, t.ExitCode, t.Reason = "failed", 1, err.Error()
		}
		r.activeStart = time.Time{}
		t.ActiveAt = time.Time{}
		if err := r.emit("task_finished", "", "", t.Reason, *t); err != nil {
			t.Status, t.ExitCode, t.Reason, runErr = "failed", 1, err.Error(), err
			t.Sequence++
			_ = r.Store.Append(Event{Version: 1, TaskID: t.ID, Sequence: t.Sequence, Time: time.Now().UTC(), Type: "delivery_failed", Message: err.Error()})
		}
		if err := r.save(); err != nil {
			runErr = err
			t.Status, t.ExitCode, t.Reason = "failed", 1, err.Error()
		}
		result = *t
	}()
	if err := r.save(); err != nil {
		return *t, err
	}
	if err := r.emit("task_started", "", "", t.Goal, nil); err != nil {
		return *t, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return *t, err
		}
		if err := r.applyUpdates(); err != nil {
			return *t, err
		}
		if t.Status == "waiting_user" || t.Checkpoint.Stage == "done" {
			return *t, nil
		}
		remaining := t.Options.Timeout - t.Elapsed
		if remaining <= 0 {
			return *t, context.DeadlineExceeded
		}
		t.Deadline = time.Now().UTC().Add(remaining)
		nodeContext, nodeCancel := context.WithTimeout(ctx, remaining)
		var err error
		if t.Checkpoint.Stage == "planning" {
			err = r.plan(nodeContext)
		} else if len(t.Plans) == 0 {
			err = errors.New("checkpoint has no accepted plan")
		} else {
			err = r.execute(nodeContext, t.Plans[len(t.Plans)-1])
		}
		nodeErr := nodeContext.Err()
		nodeCancel()
		if nodeErr != nil {
			return *t, nodeErr
		}
		if err != nil {
			return *t, err
		}
	}
}

func (r *Runner) plan(ctx context.Context) error {
	t := r.task
	before, err := r.capture("")
	if err != nil {
		return err
	}
	output, err := r.call(ctx, t.Coordinator, "coordinator", t.Cwd, coordinatorInstructions+"\nTask brief:\n"+r.brief(), before)
	if err != nil {
		return err
	}
	after, err := r.capture("")
	if err != nil {
		return err
	}
	if before != after {
		return errors.New("coordinator modified task materials")
	}
	var p Plan
	if err := decode(output, &p); err != nil {
		if e := r.Store.Write(t.ID, "coordinator.rejected.txt", []byte(output)); e != nil {
			return e
		}
		return err
	}
	if err := r.validatePlan(p); err != nil {
		if e := r.Store.Write(t.ID, "coordinator.rejected.txt", []byte(output)); e != nil {
			return e
		}
		return r.wait(err.Error())
	}
	p.Version = len(t.Plans) + 1
	t.Plans = append(t.Plans, p)
	t.Status = "running"
	t.Checkpoint.Stage, t.Checkpoint.Round, t.Checkpoint.VerifyIndex = "worker", 0, 0
	for _, action := range p.Actions {
		if action.Type == "revise_plan" {
			revision := p
			revision.Version = len(t.Plans) + 1
			revision.Reason = action.Objective
			t.Plans = append(t.Plans, revision)
			if err := r.emit("plan_revision", "", "", action.Objective, nil); err != nil {
				return err
			}
		}
	}
	if err := r.commit(); err != nil {
		return err
	}
	return r.emit("plan_accepted", "", "", p.Reason, p)
}

type Receipt struct {
	Diagnostics string              `json:"diagnostics,omitempty"`
	Snapshot    string              `json:"snapshot,omitempty"`
	Result      workflow.StepResult `json:"result"`
	CLIExitCode int                 `json:"cli_exit_code"`
}

func (r *Runner) attempt(ctx context.Context, wr workflow.Runner, id, cwd, boundary string, step workflow.Step, output bool) workflow.StepResult {
	if step.ResumeSession != "" {
		d, _ := r.Registry.Get(step.Agent)
		if d.Config.Adapter == "codex" {
			for i := 0; i+1 < len(step.Args); i++ {
				if step.Args[i] == "--sandbox" {
					step.Args[i], step.Args[i+1] = "-c", "sandbox_mode="+strconvQuote(step.Args[i+1])
				}
			}
		}
	}
	var res workflow.StepResult
	if r.Execute != nil {
		res = r.Execute(ctx, id, cwd, boundary, step, output, wr.Events)
	} else {
		res = wr.ExecuteAttempt(ctx, id, cwd, boundary, step, output)
	}
	root := r.task.Output
	if r.task.Workspace != nil {
		root = r.task.Workspace.Path
	}
	hash, snapshotErr := MaterialSnapshot(root, r.task.Kind == "project")
	if snapshotErr != nil {
		res.Status, res.CLIExitCode, res.Error = "failed", 1, snapshotErr.Error()
	}
	data, err := json.Marshal(Receipt{Result: res, CLIExitCode: res.CLIExitCode, Snapshot: hash, Diagnostics: res.Diagnostics})
	if err == nil {
		err = r.Store.Write(r.task.ID, id+".receipt.json", data)
	}
	if err != nil {
		res.Status, res.CLIExitCode, res.ExitCode, res.Error = "failed", 1, 1, err.Error()
	}
	return res
}
func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

func sessionUnavailable(res workflow.StepResult) bool {
	if res.Status == "success" || res.ParseStatus == "complete" {
		return false
	}
	text := strings.ToLower(res.Error + "\n" + res.Diagnostics)
	for _, marker := range []string{"session not found", "session has expired", "conversation not found", "no session found", "no rollout found", "thread not found"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

type heartbeat struct {
	Owner   string        `json:"owner"`
	Elapsed time.Duration `json:"elapsed_ns"`
}

func (r *Runner) recoverRuns() error {
	t := r.task
	originalElapsed, activeAt := t.Elapsed, t.ActiveAt
	if !activeAt.IsZero() && t.OwnerToken != "" {
		data, err := os.ReadFile(filepath.Join(r.Store.Dir(t.ID), "heartbeat.json"))
		if err == nil {
			var hb heartbeat
			if err := json.Unmarshal(data, &hb); err != nil {
				return err
			}
			if hb.Owner == t.OwnerToken && hb.Elapsed > t.Elapsed {
				t.Elapsed = hb.Elapsed
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	r.task.Usage = nil
	for _, a := range r.task.Attempts {
		record, err := r.Runs.Load(a.RunID)
		if err != nil {
			return err
		}
		if record.Status == "running" {
			r.task.Status = "needs_reconcile"
			data, err := os.ReadFile(filepath.Join(r.Store.Dir(r.task.ID), a.RunID+".receipt.json"))
			if err == nil {
				var receipt Receipt
				if err := json.Unmarshal(data, &receipt); err != nil {
					return err
				}
				res := receipt.Result
				record.Status, record.ExitCode, record.Error, record.ErrorKind, record.ParseStatus = res.Status, res.ExitCode, res.Error, res.ErrorKind, res.ParseStatus
				record.SessionID, record.Usage, record.EndedAt = res.SessionID, res.Usage, res.EndedAt
			} else if os.IsNotExist(err) {
				record.Status, record.ExitCode, record.Error, record.EndedAt = "orphaned", 1, "Task owner exited without a confirmed attempt result", time.Now().UTC()
				r.task.UsageIncomplete = true
			} else {
				return err
			}
			if err := r.Runs.Save(record); err != nil {
				return err
			}
		}
		if !activeAt.IsZero() && !record.EndedAt.IsZero() && record.Status != "orphaned" && record.EndedAt.After(activeAt) {
			elapsed := originalElapsed + record.EndedAt.Sub(activeAt)
			if elapsed > t.Elapsed {
				t.Elapsed = elapsed
			}
		}
		if record.Usage != nil {
			if r.task.Usage == nil {
				r.task.Usage = &agent.Usage{}
			}
			r.task.Usage.Add(record.Usage)
		}
	}
	t.ActiveAt = time.Time{}
	return nil
}

func (s *Store) Inspect(id string) (Task, error) {
	t, err := s.Load(id)
	if err != nil {
		return t, err
	}
	active, err := s.Active(id, "owner.lock")
	if err != nil {
		return t, err
	}
	t.OwnerActive = active
	if !active && (t.Status == "running" || t.Status == "planning") {
		lock, err := s.Lock(id, "owner.lock", false)
		if errors.Is(err, ErrOwned) {
			return s.Load(id)
		}
		if err != nil {
			return t, err
		}
		defer lock.Close()
		t, err = s.Load(id)
		if err != nil {
			return t, err
		}
		t.OwnerActive = false
		if err := s.Recover(&t); err != nil {
			return t, err
		}
		if t.Status == "running" || t.Status == "planning" {
			t.Status, t.ExitCode, t.Reason = "needs_reconcile", 1, "Task owner is inactive; inspect the checkpoint and uncertain operation before continuing."
			if t.Checkpoint != nil && t.Checkpoint.InFlight == "" {
				t.Status, t.ExitCode, t.Reason = "paused", 0, "Task owner is inactive at a checkpoint; continue will validate retained materials."
			}
			if err := s.Save(&t); err != nil {
				return t, err
			}
		}
	}
	c, err := s.Control(id)
	if err != nil {
		return t, err
	}
	t.Pending = nil
	for _, u := range c.Updates {
		if t.Checkpoint == nil || u.Sequence > t.Checkpoint.Applied {
			t.Pending = append(t.Pending, u)
		}
	}
	return t, nil
}

// MaterialSnapshot uses the same content and permission rules as checkpoints.
func MaterialSnapshot(root string, project bool) (string, error) {
	_, hash, err := material(root, project, "")
	return hash, err
}

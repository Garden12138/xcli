package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/task"
	"github.com/spf13/cobra"
)

func parseTaskVerify(values []string) ([][]string, error) {
	var result [][]string
	for _, text := range values {
		var argv []string
		if err := json.Unmarshal([]byte(text), &argv); err != nil || len(argv) == 0 {
			return nil, errors.New("--verify requires a nonempty JSON argv array")
		}
		result = append(result, argv)
	}
	return result, nil
}
func (a *app) taskRunner(command *cobra.Command, events bool) (*task.Runner, error) {
	cfg, _, registry, err := a.load()
	if err != nil {
		return nil, err
	}
	store, err := task.NewStore()
	if err != nil {
		return nil, err
	}
	runs, err := newStore()
	if err != nil {
		return nil, err
	}
	encoder := json.NewEncoder(command.OutOrStdout())
	var last time.Time
	r := &task.Runner{Config: cfg, Registry: registry, Store: store, Runs: runs, Sink: func(e task.Event) error {
		if events {
			return encoder.Encode(e)
		}
		if e.Type == "progress" && time.Since(last) < 250*time.Millisecond {
			return nil
		}
		last = time.Now()
		_, err := fmt.Fprintf(command.ErrOrStderr(), "[%s] %s %s %s\n", e.TaskID, e.Type, e.Role, e.Message)
		return err
	}}
	r.Execute = supervisedAttempt(r)
	return r, nil
}
func writeTaskResult(command *cobra.Command, t task.Task, err error, asJSON, events bool) error {
	if t.ID == "" {
		return err
	}
	if asJSON {
		if e := encodeJSON(command.OutOrStdout(), t); e != nil {
			return e
		}
	} else if !events {
		if e := writeTaskSummary(command, t); e != nil {
			return e
		}
	}
	if err != nil {
		return err
	}
	if t.ExitCode != 0 {
		return &ExitError{Code: t.ExitCode, Message: t.Reason}
	}
	return nil
}
func (a *app) newTaskCommand() *cobra.Command {
	opts := task.Options{Timeout: 10 * time.Minute, CallTimeout: task.DefaultCallTimeout, MaxCalls: 6}
	var asJSON, events, detach bool
	var verifies []string
	command := &cobra.Command{Use: "task <goal>", Short: "Complete a resumable Work or Project task", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		if asJSON && events {
			return errors.New("--json and --events are mutually exclusive")
		}
		if detach && events {
			return errors.New("use task status --events --follow to observe a detached Task")
		}
		if opts.CallTimeout <= 0 {
			return errors.New("call-timeout must be positive")
		}
		var err error
		opts.Goal = args[0]
		opts.Verify, err = parseTaskVerify(verifies)
		if err != nil {
			return err
		}
		r, err := a.taskRunner(command, events)
		if err != nil {
			return err
		}
		ctx, cancel := signalContext(command.Context())
		defer cancel()
		var t task.Task
		if detach {
			t, err = r.Create(ctx, opts)
			if err == nil {
				t, err = startDetachedTask(r, t.ID, task.Update{})
			}
		} else {
			t, err = r.Run(ctx, opts)
		}
		return writeTaskResult(command, t, err, asJSON, events)
	}}
	command.Flags().StringVar(&opts.Agent, "agent", "", "lock all Agent execution to this configured CLI")
	command.Flags().StringVar(&opts.Project, "project", "", "local Git project; uses an isolated worktree")
	command.Flags().StringArrayVar(&opts.Inputs, "input", nil, "input file or directory (repeatable)")
	command.Flags().StringVar(&opts.Output, "output", "", "deliverable directory")
	command.Flags().StringArrayVar(&verifies, "verify", nil, "explicit validation JSON argv array (repeatable)")
	command.Flags().DurationVar(&opts.Timeout, "timeout", 10*time.Minute, "cumulative execution time budget (excluding pauses)")
	command.Flags().DurationVar(&opts.CallTimeout, "call-timeout", task.DefaultCallTimeout, "maximum duration of each Agent CLI invocation (total --timeout still applies)")
	command.Flags().IntVar(&opts.MaxCalls, "max-calls", 6, "cumulative maximum Agent CLI invocations")
	command.Flags().BoolVar(&asJSON, "json", false, "print a single Task object")
	command.Flags().BoolVar(&events, "events", false, "stream Task JSONL events")
	command.Flags().BoolVar(&detach, "detach", false, "continue independently of this terminal")
	command.AddCommand(a.newTaskContinueCommand(), newTaskStatusCommand(), newTaskStopCommand())
	return command
}
func (a *app) newTaskContinueCommand() *cobra.Command {
	var u task.Update
	var asJSON, events, detach bool
	var verifies []string
	c := &cobra.Command{Use: "continue <task-id> [message]", Short: "Continue a Task or queue an update at its next checkpoint", Args: cobra.RangeArgs(1, 2), RunE: func(c *cobra.Command, args []string) error {
		if asJSON && events {
			return errors.New("--json and --events are mutually exclusive")
		}
		if detach && events {
			return errors.New("--detach and --events are mutually exclusive")
		}
		if c.Flags().Changed("timeout") && u.Timeout <= 0 {
			return errors.New("timeout must be positive")
		}
		if c.Flags().Changed("max-calls") && u.MaxCalls <= 0 {
			return errors.New("max-calls must be positive")
		}
		if c.Flags().Changed("call-timeout") && u.CallTimeout <= 0 {
			return errors.New("call-timeout must be positive")
		}
		if len(args) > 1 {
			u.Message = args[1]
		}
		var err error
		u.Verify, err = parseTaskVerify(verifies)
		if err != nil {
			return err
		}
		r, err := a.taskRunner(c, events)
		if err != nil {
			return err
		}
		ctx, cancel := signalContext(c.Context())
		defer cancel()
		var t task.Task
		if detach {
			t, err = startDetachedTask(r, args[0], u)
		} else {
			t, err = r.Continue(ctx, args[0], u)
		}
		if events && t.OwnerActive {
			_, e := fmt.Fprintf(c.ErrOrStderr(), "Task %s update queued; inspect pending updates with task status --json.\n", t.ID)
			if e != nil {
				return e
			}
		}

		return writeTaskResult(c, t, err, asJSON, events)
	}}
	c.Flags().StringArrayVar(&verifies, "verify", nil, "replace explicit validation JSON argv arrays")
	c.Flags().DurationVar(&u.Timeout, "timeout", 0, "raise the Task's cumulative execution time limit")
	c.Flags().DurationVar(&u.CallTimeout, "call-timeout", 0, "raise the per-invocation Agent CLI time limit")
	c.Flags().IntVar(&u.MaxCalls, "max-calls", 0, "raise the Task's cumulative invocation limit")
	c.Flags().BoolVar(&u.Reconcile, "reconcile", false, "acknowledge uncertain operations and current materials")
	c.Flags().BoolVar(&u.FreshSession, "fresh-session", false, "restore from materials without resuming a native session")
	c.Flags().BoolVar(&detach, "detach", false, "continue in the background")
	c.Flags().BoolVar(&asJSON, "json", false, "print a single Task object")
	c.Flags().BoolVar(&events, "events", false, "stream Task JSONL events")
	return c
}
func newTaskStatusCommand() *cobra.Command {
	var asJSON, events, follow bool
	var after uint64
	c := &cobra.Command{Use: "status <task-id>", Short: "Inspect a Task, its checkpoint and persisted events", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		if asJSON && events {
			return errors.New("--json and --events are mutually exclusive")
		}
		if follow && !events {
			return errors.New("--follow requires --events")
		}
		store, err := task.NewStore()
		if err != nil {
			return err
		}
		if !events {
			t, err := store.Inspect(args[0])
			if err != nil {
				return err
			}
			if asJSON {
				return encodeJSON(c.OutOrStdout(), t)
			}
			return writeTaskSummary(c, t)
		}
		ctx, cancel := signalContext(c.Context())
		defer cancel()
		encoder := json.NewEncoder(c.OutOrStdout())
		for {
			t, err := store.Inspect(args[0])
			if err != nil {
				return err
			}
			list, err := store.Events(args[0], after)
			if err != nil {
				return err
			}
			for _, e := range list {
				if err := encoder.Encode(e); err != nil {
					return err
				}
				after = e.Sequence
			}
			if !follow || !t.OwnerActive {
				return nil
			}
			timer := time.NewTimer(150 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return &ExitError{Code: 130}
			case <-timer.C:
			}
		}
	}}
	c.Flags().BoolVar(&asJSON, "json", false, "print the Task object")
	c.Flags().BoolVar(&events, "events", false, "print persisted Task JSONL events")
	c.Flags().BoolVar(&follow, "follow", false, "follow events until the Task owner exits")
	c.Flags().Uint64Var(&after, "after", 0, "only events with a greater sequence number")
	return c
}
func newTaskStopCommand() *cobra.Command {
	var asJSON bool
	var timeout time.Duration
	c := &cobra.Command{Use: "stop <task-id>", Short: "Stop the current attempt and retain a resumable checkpoint", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		store, err := task.NewStore()
		if err != nil {
			return err
		}
		ctx, cancel := signalContext(c.Context())
		defer cancel()
		t, err := waitTaskStop(ctx, store, args[0], timeout)
		if err != nil {
			return writeTaskResult(c, t, err, asJSON, false)
		}
		if asJSON {
			return encodeJSON(c.OutOrStdout(), t)
		}
		return writeTaskSummary(c, t)
	}}
	c.Flags().BoolVar(&asJSON, "json", false, "print the stopped Task object")
	c.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "maximum time to wait for the owner and attempt to exit")
	return c
}

func writeTaskSummary(command *cobra.Command, t task.Task) error {
	w := command.OutOrStdout()
	if t.Checkpoint != nil {
		if _, err := fmt.Fprintf(w, "Checkpoint: %s; owner active: %t; execution time: %s/%s\n", t.Checkpoint.Stage, t.OwnerActive, t.Elapsed.Round(time.Millisecond), t.Options.Timeout); err != nil {
			return err
		}
	}
	for _, u := range t.Pending {
		if _, err := fmt.Fprintf(w, "Pending update %d (not applied): %s\n", u.Sequence, u.Message); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(w, "Task %s: %s\nStatus: %s\nMode: %s %s\nWorkspace: %s\nOutput: %s\nAgent calls: %d/%d; repairs: %d/1\n", t.ID, t.Goal, t.Status, t.Mode, t.Agent, t.Cwd, t.Output, t.Calls, t.MaxCalls, t.Repairs); err != nil {
		return err
	}
	if t.DiffFile != "" {
		if _, err := fmt.Fprintf(w, "Diff: %s (untracked files included in artifacts)\n", t.DiffFile); err != nil {
			return err
		}
	}
	for _, check := range t.Checks {
		if _, err := fmt.Fprintf(w, "Check %s: %s %s %s\n", check.Kind, check.Status, strings.Join(check.Argv, " "), check.Detail); err != nil {
			return err
		}
	}
	for _, review := range t.Reviews {
		if _, err := fmt.Fprintf(w, "Review %s: passed=%t; findings=%s\n", review.Kind, review.Passed != nil && *review.Passed, strings.Join(review.Findings, "; ")); err != nil {
			return err
		}
	}
	if len(t.Handoffs) > 0 {
		h := t.Handoffs[len(t.Handoffs)-1]
		if _, err := fmt.Fprintf(w, "Result: %s\nUnresolved: %s\n", h.Summary, strings.Join(h.Unresolved, "; ")); err != nil {
			return err
		}
	}
	for _, artifact := range t.Artifacts {
		if artifact.Producer != "" {
			if _, err := fmt.Fprintf(w, "Artifact: %s (%s)\n", artifact.Path, artifact.SHA256); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintf(w, "%s\nInspect: xcli task status %s --json\nContinue: xcli task continue %s\n", t.Reason, t.ID, t.ID); err != nil {
		return err
	}
	return nil
}

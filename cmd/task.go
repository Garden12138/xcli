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

func (a *app) newTaskCommand() *cobra.Command {
	opts := task.Options{Timeout: 10 * time.Minute, MaxCalls: 6}
	var asJSON, events bool
	var verifies []string
	command := &cobra.Command{Use: "task <goal>", Short: "Complete a Work or Project task with bounded validation", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		if asJSON && events {
			return errors.New("--json and --events are mutually exclusive")
		}
		opts.Goal = args[0]
		for _, text := range verifies {
			var argv []string
			if err := json.Unmarshal([]byte(text), &argv); err != nil || len(argv) == 0 {
				return errors.New("--verify requires a nonempty JSON argv array")
			}
			opts.Verify = append(opts.Verify, argv)
		}
		cfg, _, registry, err := a.load()
		if err != nil {
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
		encoder := json.NewEncoder(command.OutOrStdout())
		var last time.Time
		runner := task.Runner{Config: cfg, Registry: registry, Store: store, Runs: runs, Sink: func(e task.Event) error {
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
		ctx, cancel := signalContext(command.Context())
		defer cancel()
		result, runErr := runner.Run(ctx, opts)
		if result.ID == "" {
			return runErr
		}
		if asJSON {
			if err := encoder.Encode(result); err != nil {
				return err
			}
		} else if !events {
			if err := writeTaskSummary(command, result); err != nil {
				return err
			}
		}
		if result.ExitCode != 0 {
			return &ExitError{Code: result.ExitCode, Message: result.Reason}
		}
		return runErr
	}}
	command.Flags().StringVar(&opts.Agent, "agent", "", "lock all Agent execution to this configured CLI")
	command.Flags().StringVar(&opts.Project, "project", "", "local Git project; uses an isolated worktree")
	command.Flags().StringArrayVar(&opts.Inputs, "input", nil, "input file or directory (repeatable)")
	command.Flags().StringVar(&opts.Output, "output", "", "deliverable directory (default: private Work directory or Project worktree)")
	command.Flags().StringArrayVar(&verifies, "verify", nil, "explicit validation JSON argv array (repeatable)")
	command.Flags().DurationVar(&opts.Timeout, "timeout", 10*time.Minute, "total task deadline")
	command.Flags().IntVar(&opts.MaxCalls, "max-calls", 6, "maximum Agent CLI invocations, including planning and review")
	command.Flags().BoolVar(&asJSON, "json", false, "print a single Task result object")
	command.Flags().BoolVar(&events, "events", false, "stream Task JSONL events including the final result")
	status := &cobra.Command{Use: "status <task-id>", Short: "Read a persisted task and its evidence", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		store, err := task.NewStore()
		if err != nil {
			return err
		}
		result, err := store.Load(args[0])
		if err != nil {
			return err
		}
		if asJSON {
			return encodeJSON(command.OutOrStdout(), result)
		}
		return writeTaskSummary(command, result)
	}}
	status.Flags().BoolVar(&asJSON, "json", false, "print the saved Task object")
	command.AddCommand(status)
	return command
}

func writeTaskSummary(command *cobra.Command, t task.Task) error {
	w := command.OutOrStdout()
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
	if _, err := fmt.Fprintf(w, "%s\nInspect: xcli task status %s --json\nTask continuation and background execution are planned for R2.\n", t.Reason, t.ID); err != nil {
		return err
	}
	return nil
}

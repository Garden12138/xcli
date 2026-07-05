package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Garden12138/xcli/internal/jobs"
	"github.com/spf13/cobra"
)

func (a *app) newJobsCommand() *cobra.Command {
	command := &cobra.Command{Use: "jobs", Short: "Inspect and control background runs"}
	command.AddCommand(
		a.newJobsListCommand(), a.newJobsShowCommand(), a.newJobsLogsCommand(), a.newJobsStopCommand(),
		a.newJobsWaitCommand(), a.newJobsDeleteCommand(), a.newJobsPruneCommand(),
	)
	return command
}

func (a *app) newJobsListCommand() *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List background jobs",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			manager, err := newJobManager()
			if err != nil {
				return err
			}
			records, err := manager.List()
			if err != nil {
				return err
			}
			views := make([]jobs.View, 0, len(records))
			for _, record := range records {
				views = append(views, jobs.ToView(record))
			}
			if asJSON {
				return encodeJSON(command.OutOrStdout(), views)
			}
			writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "ID\tKIND\tAGENT/WORKFLOW\tSTATUS\tPID\tSTARTED\tENDED")
			for _, view := range views {
				ended := "-"
				if view.EndedAt != nil && !view.EndedAt.IsZero() {
					ended = view.EndedAt.Local().Format(time.RFC3339)
				}
				label := view.Agent
				if label == "" {
					label = view.Workflow
				}
				fmt.Fprintf(
					writer, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
					view.ID, view.Kind, label, view.Status, view.PID,
					view.StartedAt.Local().Format(time.RFC3339), ended,
				)
			}
			return writer.Flush()
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	return command
}

func (a *app) newJobsWaitCommand() *cobra.Command {
	var timeout time.Duration
	var asJSON bool
	command := &cobra.Command{
		Use:   "wait <run-id>",
		Short: "Wait for a background job to finish",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if timeout < 0 {
				return errors.New("timeout cannot be negative")
			}
			manager, err := newJobManager()
			if err != nil {
				return err
			}
			ctx, cancel := signalContext(command.Context())
			defer cancel()
			var timer *time.Timer
			var deadline <-chan time.Time
			if timeout > 0 {
				timer = time.NewTimer(timeout)
				defer timer.Stop()
				deadline = timer.C
			}
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
			for {
				record, err := manager.Load(args[0])
				if err != nil {
					return err
				}
				view := jobs.ToView(record)
				if jobs.IsTerminal(record.Status) {
					if err := writeWaitedJob(command, view, asJSON); err != nil {
						return err
					}
					if record.ExitCode != 0 {
						return &ExitError{Code: record.ExitCode}
					}
					return nil
				}
				select {
				case <-ctx.Done():
					if err := writeWaitedJob(command, view, asJSON); err != nil {
						return err
					}
					return &ExitError{Code: 130, Message: fmt.Sprintf("interrupted while waiting for job %s", record.ID)}
				case <-deadline:
					if err := writeWaitedJob(command, view, asJSON); err != nil {
						return err
					}
					return &ExitError{Code: 124, Message: fmt.Sprintf("timed out waiting for job %s", record.ID)}
				case <-ticker.C:
				}
			}
		},
	}
	command.Flags().DurationVar(&timeout, "timeout", 0, "maximum time to wait (zero waits indefinitely)")
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	return command
}

func (a *app) newJobsShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <run-id>",
		Short: "Show a background job as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			manager, err := newJobManager()
			if err != nil {
				return err
			}
			record, err := manager.Load(args[0])
			if err != nil {
				return err
			}
			return encodeJSON(command.OutOrStdout(), jobs.ToView(record))
		},
	}
}

func (a *app) newJobsLogsCommand() *cobra.Command {
	var follow bool
	command := &cobra.Command{
		Use:   "logs <run-id>",
		Short: "Print normalized background job logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			manager, err := newJobManager()
			if err != nil {
				return err
			}
			record, err := manager.Load(args[0])
			if err != nil {
				return err
			}
			path, err := manager.LogPath(record.ID)
			if err != nil {
				return err
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			if !follow {
				_, err = io.Copy(command.OutOrStdout(), file)
				return err
			}
			ctx, cancel := signalContext(command.Context())
			defer cancel()
			for {
				if _, err := io.Copy(command.OutOrStdout(), file); err != nil {
					return err
				}
				record, err = manager.Load(record.ID)
				if err != nil {
					return err
				}
				if jobs.IsTerminal(record.Status) {
					_, err = io.Copy(command.OutOrStdout(), file)
					return err
				}
				timer := time.NewTimer(200 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return &ExitError{Code: 130}
				case <-timer.C:
				}
			}
		},
	}
	command.Flags().BoolVar(&follow, "follow", false, "follow logs until the job finishes")
	return command
}

func (a *app) newJobsStopCommand() *cobra.Command {
	var timeout time.Duration
	var force bool
	var asJSON bool
	command := &cobra.Command{
		Use:   "stop <run-id>",
		Short: "Stop a background job",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if timeout <= 0 {
				return errors.New("timeout must be greater than zero")
			}
			manager, err := newJobManager()
			if err != nil {
				return err
			}
			record, err := manager.Load(args[0])
			if err != nil {
				return err
			}
			if jobs.IsTerminal(record.Status) {
				return writeStoppedJob(command, jobs.ToView(record), asJSON)
			}
			held, err := manager.LockHeld(record.ID)
			if err != nil {
				return err
			}
			if !held {
				record, err = manager.Reconcile(record)
				if err != nil {
					return err
				}
				return writeStoppedJob(command, jobs.ToView(record), asJSON)
			}
			if err := jobs.Terminate(record.PID, force); err != nil {
				return err
			}
			if force {
				record, err = manager.MarkKilled(record)
				if err != nil {
					return err
				}
				return writeStoppedJob(command, jobs.ToView(record), asJSON)
			}

			ctx, cancel := signalContext(command.Context())
			defer cancel()
			deadline := time.NewTimer(timeout)
			defer deadline.Stop()
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return &ExitError{Code: 130}
				case <-ticker.C:
					record, err = manager.Load(record.ID)
					if err != nil {
						return err
					}
					if jobs.IsTerminal(record.Status) {
						return writeStoppedJob(command, jobs.ToView(record), asJSON)
					}
				case <-deadline.C:
					if err := jobs.Terminate(record.PID, true); err != nil {
						return err
					}
					record, err = manager.MarkKilled(record)
					if err != nil {
						return err
					}
					return writeStoppedJob(command, jobs.ToView(record), asJSON)
				}
			}
		},
	}
	command.Flags().DurationVar(&timeout, "timeout", 5*time.Second, "grace period before force killing")
	command.Flags().BoolVar(&force, "force", false, "kill immediately")
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	return command
}

type jobCleanupResult struct {
	DryRun     bool              `json:"dry_run"`
	Candidates []jobs.View       `json:"candidates"`
	Deleted    []string          `json:"deleted,omitempty"`
	Errors     map[string]string `json:"errors,omitempty"`
}

func (a *app) newJobsDeleteCommand() *cobra.Command {
	var yes bool
	var asJSON bool
	command := &cobra.Command{
		Use:   "delete <run-id>",
		Short: "Delete a finished background job and its artifacts",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if asJSON && !yes {
				return errors.New("--json requires --yes for job deletion")
			}
			manager, err := newJobManager()
			if err != nil {
				return err
			}
			record, err := manager.Load(args[0])
			if err != nil {
				return err
			}
			if !jobs.IsTerminal(record.Status) {
				return fmt.Errorf("job %q is still %s; stop it before deleting", record.ID, record.Status)
			}
			view := jobs.ToView(record)
			if !asJSON {
				if err := writeJobCleanupPlan(command, []jobs.View{view}); err != nil {
					return err
				}
			}
			confirmed, err := confirmJobCleanup(command, yes, "Delete this job and its artifacts? [y/N] ")
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Fprintln(command.OutOrStdout(), "Canceled.")
				return nil
			}
			if _, err := manager.Delete(record.ID); err != nil {
				return err
			}
			result := jobCleanupResult{Candidates: []jobs.View{view}, Deleted: []string{record.ID}}
			if asJSON {
				return encodeJSON(command.OutOrStdout(), result)
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Deleted job %s.\n", record.ID)
			return err
		},
	}
	command.Flags().BoolVarP(&yes, "yes", "y", false, "delete without a confirmation prompt")
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	return command
}

func (a *app) newJobsPruneCommand() *cobra.Command {
	var olderThan time.Duration
	var dryRun bool
	var yes bool
	var asJSON bool
	command := &cobra.Command{
		Use:   "prune",
		Short: "Delete finished background jobs older than a duration",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if !command.Flags().Changed("older-than") || olderThan <= 0 {
				return errors.New("--older-than must be provided and greater than zero")
			}
			if asJSON && !dryRun && !yes {
				return errors.New("--json requires --dry-run or --yes for job pruning")
			}
			manager, err := newJobManager()
			if err != nil {
				return err
			}
			records, err := manager.PruneCandidates(time.Now().UTC().Add(-olderThan))
			if err != nil {
				return err
			}
			views := make([]jobs.View, 0, len(records))
			for _, record := range records {
				views = append(views, jobs.ToView(record))
			}
			result := jobCleanupResult{DryRun: dryRun, Candidates: views}
			if len(records) == 0 {
				if asJSON {
					return encodeJSON(command.OutOrStdout(), result)
				}
				fmt.Fprintln(command.OutOrStdout(), "No finished jobs matched the prune criteria.")
				return nil
			}
			if !asJSON {
				if err := writeJobCleanupPlan(command, views); err != nil {
					return err
				}
			}
			if dryRun {
				if asJSON {
					return encodeJSON(command.OutOrStdout(), result)
				}
				fmt.Fprintln(command.OutOrStdout(), "Dry run; no jobs were deleted.")
				return nil
			}
			confirmed, err := confirmJobCleanup(command, yes, "Delete these jobs and their artifacts? [y/N] ")
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Fprintln(command.OutOrStdout(), "Canceled.")
				return nil
			}
			for _, record := range records {
				if _, err := manager.Delete(record.ID); err != nil {
					if result.Errors == nil {
						result.Errors = map[string]string{}
					}
					result.Errors[record.ID] = err.Error()
					continue
				}
				result.Deleted = append(result.Deleted, record.ID)
			}
			if asJSON {
				if err := encodeJSON(command.OutOrStdout(), result); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(command.OutOrStdout(), "Deleted %d job(s).\n", len(result.Deleted))
				ids := make([]string, 0, len(result.Errors))
				for id := range result.Errors {
					ids = append(ids, id)
				}
				sort.Strings(ids)
				for _, id := range ids {
					fmt.Fprintf(command.ErrOrStderr(), "Failed to delete %s: %s\n", id, result.Errors[id])
				}
			}
			if len(result.Errors) > 0 {
				return &ExitError{Code: 1, Message: "one or more jobs could not be deleted"}
			}
			return nil
		},
	}
	command.Flags().DurationVar(&olderThan, "older-than", 0, "delete jobs that ended more than this duration ago")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "preview without deleting jobs")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "prune without a confirmation prompt")
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	return command
}

func newJobManager() (jobs.Manager, error) {
	store, err := newStore()
	if err != nil {
		return jobs.Manager{}, err
	}
	return jobs.Manager{Store: store}, nil
}

func writeStoppedJob(command *cobra.Command, view jobs.View, asJSON bool) error {
	if asJSON {
		return encodeJSON(command.OutOrStdout(), view)
	}
	_, err := fmt.Fprintf(command.OutOrStdout(), "Job %s: %s\n", view.ID, view.Status)
	return err
}

func writeWaitedJob(command *cobra.Command, view jobs.View, asJSON bool) error {
	if asJSON {
		return encodeJSON(command.OutOrStdout(), view)
	}
	_, err := fmt.Fprintf(command.OutOrStdout(), "Job %s: %s\n", view.ID, view.Status)
	return err
}

func writeJobCleanupPlan(command *cobra.Command, views []jobs.View) error {
	writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "ID\tKIND\tSTATUS\tENDED"); err != nil {
		return err
	}
	for _, view := range views {
		ended := "-"
		if view.EndedAt != nil && !view.EndedAt.IsZero() {
			ended = view.EndedAt.Local().Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", view.ID, view.Kind, view.Status, ended); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func confirmJobCleanup(command *cobra.Command, yes bool, prompt string) (bool, error) {
	if yes {
		return true, nil
	}
	input := command.InOrStdin()
	file, ok := input.(*os.File)
	if !ok || !isTerminal(file) {
		return false, errors.New("refusing to delete jobs without a terminal; pass --yes to confirm")
	}
	fmt.Fprint(command.OutOrStdout(), prompt)
	answer, _ := bufio.NewReader(input).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

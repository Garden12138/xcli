package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/Garden12138/xcli/internal/jobs"
	"github.com/spf13/cobra"
)

func (a *app) newJobsCommand() *cobra.Command {
	command := &cobra.Command{Use: "jobs", Short: "Inspect and control background runs"}
	command.AddCommand(a.newJobsListCommand(), a.newJobsShowCommand(), a.newJobsLogsCommand(), a.newJobsStopCommand())
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
			fmt.Fprintln(writer, "ID\tAGENT\tSTATUS\tPID\tSTARTED\tENDED")
			for _, view := range views {
				ended := "-"
				if view.EndedAt != nil && !view.EndedAt.IsZero() {
					ended = view.EndedAt.Local().Format(time.RFC3339)
				}
				fmt.Fprintf(
					writer, "%s\t%s\t%s\t%d\t%s\t%s\n",
					view.ID, view.Agent, view.Status, view.PID,
					view.StartedAt.Local().Format(time.RFC3339), ended,
				)
			}
			return writer.Flush()
		},
	}
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

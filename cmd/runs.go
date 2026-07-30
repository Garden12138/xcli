package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/Garden12138/xcli/internal/runstore"
	"github.com/spf13/cobra"
)

func (a *app) newRunsCommand() *cobra.Command {
	runs := &cobra.Command{Use: "runs", Short: "Inspect xcli run metadata"}
	runs.AddCommand(a.newRunsListCommand(), a.newRunsShowCommand())
	return runs
}

func (a *app) newRunsListCommand() *cobra.Command {
	var asJSON bool
	var asJSONL bool
	var selectedAgent string
	var kind string
	var status string
	var cwd string
	var sinceValue string
	command := &cobra.Command{
		Use:   "list",
		Short: "List recorded runs",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if asJSON && asJSONL {
				return errors.New("--json and --jsonl cannot be combined")
			}
			store, err := newStore()
			if err != nil {
				return err
			}
			records, err := store.List()
			if err != nil {
				return err
			}
			var since *time.Time
			if sinceValue != "" {
				since, err = parseRunSince(sinceValue, time.Now())
				if err != nil {
					return err
				}
			}
			if cwd != "" {
				cwd, err = filepath.Abs(cwd)
				if err != nil {
					return err
				}
			}
			records = runstore.Filter(records, runstore.FilterOptions{
				Agent: selectedAgent, Kind: kind, Status: status, Cwd: cwd, Since: since,
			})
			if asJSON {
				return encodeJSON(command.OutOrStdout(), records)
			}
			if asJSONL {
				encoder := json.NewEncoder(command.OutOrStdout())
				for _, record := range records {
					if err := encoder.Encode(record); err != nil {
						return err
					}
				}
				return nil
			}
			writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "ID\tKIND\tAGENT/WORKFLOW\tSTATUS\tSTARTED")
			for _, record := range records {
				label := record.Agent
				if label == "" {
					label = record.Workflow
				}
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", record.ID, record.Kind, label, record.Status, record.StartedAt.Local().Format(time.RFC3339))
			}
			return writer.Flush()
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	command.Flags().BoolVar(&asJSONL, "jsonl", false, "print one JSON run record per line")
	command.Flags().StringVar(&selectedAgent, "agent", "", "include runs that used one agent")
	command.Flags().StringVar(&kind, "kind", "", "include one run kind")
	command.Flags().StringVar(&status, "status", "", "include one run status")
	command.Flags().StringVar(&cwd, "cwd", "", "include runs with this exact working directory")
	command.Flags().StringVar(&sinceValue, "since", "", "include runs since an RFC3339 time or duration such as 24h")
	return command
}

func parseRunSince(value string, now time.Time) (*time.Time, error) {
	if duration, err := time.ParseDuration(value); err == nil {
		if duration < 0 {
			return nil, errors.New("--since duration cannot be negative")
		}
		result := now.UTC().Add(-duration)
		return &result, nil
	}
	result, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("invalid --since %q; use RFC3339 or a duration such as 24h", value)
	}
	result = result.UTC()
	return &result, nil
}

func (a *app) newRunsShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <run-id>",
		Short: "Show a run record as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			record, err := store.Load(args[0])
			if err != nil {
				return err
			}
			return encodeJSON(command.OutOrStdout(), record)
		},
	}
}

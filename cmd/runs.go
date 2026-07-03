package cmd

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

func (a *app) newRunsCommand() *cobra.Command {
	runs := &cobra.Command{Use: "runs", Short: "Inspect xcli run metadata"}
	runs.AddCommand(a.newRunsListCommand(), a.newRunsShowCommand())
	return runs
}

func (a *app) newRunsListCommand() *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List recorded runs",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			records, err := store.List()
			if err != nil {
				return err
			}
			if asJSON {
				return encodeJSON(command.OutOrStdout(), records)
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
	return command
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

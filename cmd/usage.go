package cmd

import (
	"fmt"
	"strconv"
	"text/tabwriter"

	usagereport "github.com/Garden12138/xcli/internal/usage"
	"github.com/spf13/cobra"
)

func (a *app) newUsageCommand() *cobra.Command {
	var days int
	var selectedAgent string
	var asJSON bool
	command := &cobra.Command{
		Use:   "usage",
		Short: "Summarize recorded token usage and estimated costs",
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
			report, err := usagereport.Build(records, usagereport.Options{Days: days, Agent: selectedAgent})
			if err != nil {
				return err
			}
			if asJSON {
				return encodeJSON(command.OutOrStdout(), report)
			}
			return writeUsageTable(command, report)
		},
	}
	command.Flags().IntVar(&days, "days", 0, "include only the most recent N days (0 means all time)")
	command.Flags().StringVar(&selectedAgent, "agent", "", "include only one agent")
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	return command
}

func writeUsageTable(command *cobra.Command, report usagereport.Report) error {
	writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "AGENT\tTASKS\tTRACKED\tINPUT\tCACHE_READ\tCACHE_WRITE\tOUTPUT\tREASONING\tTOTAL\tEST_COST_USD\tCOSTED")
	for _, summary := range report.ByAgent {
		writeUsageRow(writer, summary.Agent, summary)
	}
	writeUsageRow(writer, "TOTAL", report.Totals)
	return writer.Flush()
}

func writeUsageRow(writer *tabwriter.Writer, label string, summary usagereport.Summary) {
	tracked := fmt.Sprintf("%d/%d", summary.TrackedTasks, summary.Tasks)
	costed := fmt.Sprintf("%d/%d", summary.CostedTasks, summary.Tasks)
	cost := "-"
	if summary.Usage.EstimatedCostUSD != nil {
		cost = strconv.FormatFloat(*summary.Usage.EstimatedCostUSD, 'f', 6, 64)
	}
	fmt.Fprintf(
		writer, "%s\t%d\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%s\t%s\n",
		label, summary.Tasks, tracked, summary.Usage.InputTokens, summary.Usage.CacheReadTokens,
		summary.Usage.CacheWriteTokens, summary.Usage.OutputTokens, summary.Usage.ReasoningTokens,
		summary.Usage.TotalTokens, cost, costed,
	)
}

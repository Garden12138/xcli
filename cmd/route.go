package cmd

import (
	"fmt"
	"strings"

	"github.com/Garden12138/xcli/internal/routing"
	"github.com/spf13/cobra"
)

func (a *app) newRouteCommand() *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "route <prompt>",
		Short: "Explain which agent would handle a prompt",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			cfg, _, _, err := a.load()
			if err != nil {
				return err
			}
			decision, err := routing.Select(cfg, strings.Join(args, " "))
			if err != nil {
				return err
			}
			if asJSON {
				return encodeJSON(command.OutOrStdout(), decision)
			}
			fmt.Fprintf(command.OutOrStdout(), "Agent: %s\n", decision.Agent)
			fmt.Fprintf(command.OutOrStdout(), "Source: %s\n", decision.Source)
			if decision.Rule != "" {
				fmt.Fprintf(command.OutOrStdout(), "Rule: %s\n", decision.Rule)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	return command
}

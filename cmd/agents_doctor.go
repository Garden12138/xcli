package cmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/spf13/cobra"
)

func (a *app) newAgentsCommand() *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "agents",
		Short: "List configured agents",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, _, registry, err := a.load()
			if err != nil {
				return err
			}
			infos := make([]agent.Info, 0, len(registry.Names()))
			for _, name := range registry.Names() {
				definition, _ := registry.Get(name)
				installation := definition.Detect(command.Context())
				infos = append(infos, agent.Info{
					Name: name, Adapter: definition.Config.Adapter, Command: definition.Config.Command,
					Network: definition.Config.Network, Installed: installation.Installed,
					Path: installation.Path, Version: installation.Version,
				})
			}
			if asJSON {
				return encodeJSON(command.OutOrStdout(), infos)
			}
			writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "NAME\tADAPTER\tINSTALLED\tNETWORK\tVERSION")
			for _, info := range infos {
				fmt.Fprintf(writer, "%s\t%s\t%t\t%s\t%s\n", info.Name, info.Adapter, info.Installed, fallback(info.Network, "inherit"), info.Version)
			}
			return writer.Flush()
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	return command
}

func (a *app) newDoctorCommand() *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "doctor [agent]",
		Short: "Check configuration and agent installations",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			cfg, path, registry, err := a.load()
			if err != nil {
				return err
			}
			names := registry.Names()
			if len(args) == 1 {
				if !registry.Has(args[0]) {
					return fmt.Errorf("unknown agent %q", args[0])
				}
				names = args
			}
			type report struct {
				Config       string                        `json:"config"`
				DefaultAgent string                        `json:"default_agent,omitempty"`
				Agents       map[string]agent.Installation `json:"agents"`
			}
			result := report{Config: path, DefaultAgent: cfg.DefaultAgent, Agents: map[string]agent.Installation{}}
			allInstalled := true
			for _, name := range names {
				definition, _ := registry.Get(name)
				installation := definition.Detect(context.Background())
				result.Agents[name] = installation
				allInstalled = allInstalled && installation.Installed
			}
			if asJSON {
				if err := encodeJSON(command.OutOrStdout(), result); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(command.OutOrStdout(), "Config: %s\n", path)
				fmt.Fprintf(command.OutOrStdout(), "Default agent: %s\n", fallback(cfg.DefaultAgent, "not set"))
				writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
				fmt.Fprintln(writer, "AGENT\tSTATUS\tPATH\tVERSION")
				for _, name := range names {
					installation := result.Agents[name]
					status := "missing"
					if installation.Installed {
						status = "ok"
					}
					fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", name, status, installation.Path, installation.Version)
				}
				if err := writer.Flush(); err != nil {
					return err
				}
			}
			if len(args) == 1 && !allInstalled {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	return command
}

func fallback(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

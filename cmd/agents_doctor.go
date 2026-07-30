package cmd

import (
	"fmt"
	"os/exec"
	"text/tabwriter"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/spf13/cobra"
)

type doctorAgentReport struct {
	agent.Installation
	Capabilities *agent.Capabilities `json:"capabilities,omitempty"`
	ACPCommand   string              `json:"acp_command,omitempty"`
	ACPAvailable *bool               `json:"acp_available,omitempty"`
	ACPError     string              `json:"acp_error,omitempty"`
}

type doctorReport struct {
	Config       string                       `json:"config"`
	DefaultAgent string                       `json:"default_agent,omitempty"`
	Deep         bool                         `json:"deep,omitempty"`
	Agents       map[string]doctorAgentReport `json:"agents"`
}

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
	var deep bool
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
			result := doctorReport{
				Config: path, DefaultAgent: cfg.DefaultAgent, Deep: deep,
				Agents: map[string]doctorAgentReport{},
			}
			allHealthy := true
			for _, name := range names {
				definition, _ := registry.Get(name)
				installation := definition.Detect(command.Context())
				item := doctorAgentReport{Installation: installation}
				if deep {
					capabilities := definition.Capabilities()
					item.Capabilities = &capabilities
					acpAvailable := false
					if capabilities.ACP {
						launch, launchErr := definition.ACP(nil)
						if launchErr != nil {
							item.ACPError = launchErr.Error()
						} else {
							item.ACPCommand = launch.Command
							if _, lookErr := exec.LookPath(launch.Command); lookErr != nil {
								item.ACPError = lookErr.Error()
							} else {
								acpAvailable = true
							}
						}
					}
					item.ACPAvailable = &acpAvailable
				}
				result.Agents[name] = item
				healthy := installation.Installed
				if deep && item.Capabilities.ACP {
					healthy = healthy && item.ACPAvailable != nil && *item.ACPAvailable
				}
				allHealthy = allHealthy && healthy
			}
			if asJSON {
				if err := encodeJSON(command.OutOrStdout(), result); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(command.OutOrStdout(), "Config: %s\n", path)
				fmt.Fprintf(command.OutOrStdout(), "Default agent: %s\n", fallback(cfg.DefaultAgent, "not set"))
				writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
				if deep {
					fmt.Fprintln(writer, "AGENT\tSTATUS\tVERSION\tRUN\tSTRUCTURED\tRESUME\tACP\tUSAGE")
				} else {
					fmt.Fprintln(writer, "AGENT\tSTATUS\tPATH\tVERSION")
				}
				for _, name := range names {
					item := result.Agents[name]
					installation := item.Installation
					status := "missing"
					if installation.Installed {
						status = "ok"
					}
					acpAvailable := item.ACPAvailable != nil && *item.ACPAvailable
					if deep && item.Capabilities.ACP && !acpAvailable {
						status = "degraded"
					}
					if deep {
						fmt.Fprintf(
							writer, "%s\t%s\t%s\t%t\t%t\t%t\t%t\t%t\n",
							name, status, installation.Version, item.Capabilities.Run,
							item.Capabilities.StructuredOutput, item.Capabilities.Resume,
							acpAvailable, item.Capabilities.Usage,
						)
					} else {
						fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", name, status, installation.Path, installation.Version)
					}
				}
				if err := writer.Flush(); err != nil {
					return err
				}
			}
			if len(args) == 1 && !allHealthy {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	command.Flags().BoolVar(&deep, "deep", false, "check configured capabilities and ACP command availability")
	return command
}

func fallback(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

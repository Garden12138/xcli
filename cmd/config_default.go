package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/Garden12138/xcli/internal/config"
	"github.com/spf13/cobra"
)

func (a *app) newDefaultCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "default [agent]",
		Short: "Show or set the default agent",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			cfg, path, registry, err := a.load()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				if cfg.DefaultAgent == "" {
					return errors.New("no default agent is configured")
				}
				fmt.Fprintln(command.OutOrStdout(), cfg.DefaultAgent)
				return nil
			}
			if !registry.Has(args[0]) {
				return fmt.Errorf("unknown agent %q", args[0])
			}
			cfg.DefaultAgent = args[0]
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "Default agent set to %s\n", args[0])
			return nil
		},
	}
}

func (a *app) newConfigCommand() *cobra.Command {
	configCommand := &cobra.Command{Use: "config", Short: "Manage xcli configuration"}
	configCommand.AddCommand(a.newConfigPathCommand(), a.newConfigValidateCommand(), a.newConfigInitCommand())
	return configCommand
}

func (a *app) newConfigPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the active configuration path",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			path, err := config.ResolvePath(a.configPath)
			if err != nil {
				return err
			}
			fmt.Fprintln(command.OutOrStdout(), path)
			return nil
		},
	}
}

func (a *app) newConfigValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate the active configuration",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, path, _, err := a.load()
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "Config is valid: %s\n", path)
			return nil
		},
	}
}

func (a *app) newConfigInitCommand() *cobra.Command {
	var force bool
	command := &cobra.Command{
		Use:   "init",
		Short: "Create a default configuration file",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			path, err := config.ResolvePath(a.configPath)
			if err != nil {
				return err
			}
			if _, err := os.Stat(path); err == nil && !force {
				return fmt.Errorf("config already exists at %s (pass --force to replace it)", path)
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			cfg := config.Defaults()
			cfg.DefaultAgent = "codex"
			cfg.Networks["direct"] = config.Network{Unset: []string{
				"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy",
			}}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "Created %s\n", path)
			return nil
		},
	}
	command.Flags().BoolVar(&force, "force", false, "replace an existing configuration file")
	return command
}

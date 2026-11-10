package cmd

import (
	"errors"
	"fmt"
	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/launch"
	"github.com/Garden12138/xcli/internal/mcp"
	xruntime "github.com/Garden12138/xcli/internal/runtime"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
)

func (a *app) newMCPCommand() *cobra.Command {
	command := &cobra.Command{Use: "mcp", Short: "Compatibility launcher for existing native MCP configuration", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		return errors.New("mcp supports only the compatibility serve launcher")
	}}
	command.AddCommand(a.newMCPServeCommand())
	return command
}
func (a *app) newMCPServeCommand() *cobra.Command {
	var projectConfig string
	command := &cobra.Command{
		Use:   "serve <server>",
		Short: "Run a configured stdio MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			var cfg config.Config
			var path string
			if projectConfig != "" {
				if a.configPath != "" {
					return errors.New("--project-config cannot be combined with --config")
				}
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				path, err = mcp.ResolveProjectConfig(cwd, projectConfig)
				if err != nil {
					return err
				}
				cfg, _, err = config.Load(path)
				if err != nil {
					return err
				}
			} else {
				var err error
				cfg, path, _, err = a.load()
				if err != nil {
					return err
				}
			}
			spec, err := mcp.BuildServeSpec(cfg, path, args[0], os.Environ())
			if err != nil {
				return err
			}
			if _, err := exec.LookPath(spec.Command.Command); err != nil {
				return fmt.Errorf("MCP server command %q was not found", spec.Command.Command)
			}
			ctx, cancel := signalContext(command.Context())
			defer cancel()
			opts := xruntime.ProcessOptions{}
			if d, ok := agent.NewRegistry(cfg).MatchCommand(spec.Command.Command); ok {
				opts.Launch = &launch.Metadata{Agent: d.Name, Adapter: d.Config.Adapter, Kind: "mcp-compat"}
			}
			opts.Dir = spec.Dir
			opts.Env = spec.Env
			opts.Stdin = command.InOrStdin()
			opts.Stdout = command.OutOrStdout()
			opts.Stderr = command.ErrOrStderr()
			result, err := xruntime.RunProcess(ctx, spec.Command, opts)

			if err != nil {
				return err
			}
			if code := processCLIExit(result); code != 0 {
				return &ExitError{Code: code}
			}
			return nil
		},
	}
	command.Flags().StringVar(&projectConfig, "project-config", "", "project-relative xcli configuration path")
	return command
}

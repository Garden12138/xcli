package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	xruntime "github.com/Garden12138/xcli/internal/runtime"
	"github.com/spf13/cobra"
)

func (a *app) newACPCommand() *cobra.Command {
	var cwdFlag string
	command := &cobra.Command{
		Use:   "acp [agent] [-- native-args...]",
		Short: "Start an agent's ACP server over stdio",
		Args: func(command *cobra.Command, args []string) error {
			before, _ := splitNativeArgs(command, args)
			if len(before) > 1 {
				return errors.New("acp accepts at most one agent before --")
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			cfg, _, registry, err := a.load()
			if err != nil {
				return err
			}
			before, nativeArgs := splitNativeArgs(command, args)
			name := cfg.DefaultAgent
			if len(before) == 1 {
				name = before[0]
			}
			if name == "" {
				return errors.New("no agent selected; pass an agent or configure xcli default")
			}
			definition, err := registry.Get(name)
			if err != nil {
				return err
			}
			launch, err := definition.ACP(nativeArgs)
			if err != nil {
				return err
			}
			if _, err := exec.LookPath(launch.Command); err != nil {
				if launch.InstallHint != "" {
					return fmt.Errorf("ACP command %q for agent %q was not found; install it with: %s", launch.Command, name, launch.InstallHint)
				}
				return fmt.Errorf("ACP command %q for agent %q was not found", launch.Command, name)
			}
			cwd, err := resolveCwd(cwdFlag)
			if err != nil {
				return err
			}
			environment, err := xruntime.BuildEnvironment(os.Environ(), cfg, definition.Config, "")
			if err != nil {
				return err
			}
			ctx, cancel := signalContext(command.Context())
			defer cancel()
			result, runErr := xruntime.RunProcess(ctx, launch.CommandSpec, xruntime.ProcessOptions{
				Dir: cwd, Env: environment, Stdin: command.InOrStdin(), Stdout: command.OutOrStdout(),
				Stderr: command.ErrOrStderr(),
			})
			if runErr != nil {
				return runErr
			}
			if result.ExitCode != 0 {
				return &ExitError{Code: result.ExitCode}
			}
			return nil
		},
	}
	command.Flags().StringVar(&cwdFlag, "cwd", "", "working directory")
	return command
}

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	xruntime "github.com/Garden12138/xcli/internal/runtime"
	"github.com/spf13/cobra"
)

func (a *app) newInstallCommand() *cobra.Command {
	var method string
	var dryRun bool
	var yes bool
	command := &cobra.Command{
		Use:   "install <agent>",
		Short: "Install an agent with npm or Homebrew",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			_, _, registry, err := a.load()
			if err != nil {
				return err
			}
			definition, err := registry.Get(args[0])
			if err != nil {
				return err
			}
			if installation := definition.Detect(command.Context()); installation.Installed && !dryRun {
				fmt.Fprintf(command.OutOrStdout(), "%s is already installed at %s\n", args[0], installation.Path)
				return nil
			}
			spec, err := definition.Install(method)
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "Command: %s\n", formatCommand(spec.Command, spec.Args))
			if dryRun {
				return nil
			}
			if !yes {
				if !isTerminal(os.Stdin) {
					return fmt.Errorf("refusing to install without a terminal; pass --yes to confirm")
				}
				fmt.Fprint(command.OutOrStdout(), "Continue? [y/N] ")
				answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "y" && answer != "yes" {
					fmt.Fprintln(command.OutOrStdout(), "Canceled.")
					return nil
				}
			}
			ctx, cancel := signalContext(command.Context())
			defer cancel()
			result, err := xruntime.RunProcess(ctx, structToCommand(spec.Command, spec.Args), xruntime.ProcessOptions{
				Env: os.Environ(), Stdin: os.Stdin, Stdout: command.OutOrStdout(), Stderr: command.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			if result.ExitCode != 0 {
				return &ExitError{Code: result.ExitCode}
			}
			return nil
		},
	}
	command.Flags().StringVar(&method, "method", "auto", "install method: auto, npm, or brew")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "print the install command without running it")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "run without a confirmation prompt")
	return command
}

func (a *app) newAuthCommand() *cobra.Command {
	auth := &cobra.Command{Use: "auth", Short: "Use an agent's native authentication flow"}
	login := &cobra.Command{
		Use:   "login <agent>",
		Short: "Log in with the native agent CLI",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			cfg, _, registry, err := a.load()
			if err != nil {
				return err
			}
			definition, err := registry.Get(args[0])
			if err != nil {
				return err
			}
			spec, err := definition.Auth()
			if err != nil {
				return err
			}
			environment, err := xruntime.BuildEnvironment(os.Environ(), cfg, definition.Config, "")
			if err != nil {
				return err
			}
			ctx, cancel := signalContext(command.Context())
			defer cancel()
			result, err := xruntime.RunProcess(ctx, spec, xruntime.ProcessOptions{
				Env: environment, Stdin: os.Stdin, Stdout: command.OutOrStdout(), Stderr: command.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			if result.ExitCode != 0 {
				return &ExitError{Code: result.ExitCode}
			}
			return nil
		},
	}
	auth.AddCommand(login)
	return auth
}

func formatCommand(command string, args []string) string {
	parts := []string{quoteArgument(command)}
	for _, arg := range args {
		parts = append(parts, quoteArgument(arg))
	}
	return strings.Join(parts, " ")
}

func quoteArgument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n\"'") {
		return value
	}
	return strconv.Quote(value)
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

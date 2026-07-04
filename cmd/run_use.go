package cmd

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/routing"
	"github.com/Garden12138/xcli/internal/runstore"
	xruntime "github.com/Garden12138/xcli/internal/runtime"
	"github.com/spf13/cobra"
)

func (a *app) newUseCommand() *cobra.Command {
	var cwdFlag string
	command := &cobra.Command{
		Use:   "use [agent] [-- native-args...]",
		Short: "Start an interactive agent session",
		Args: func(command *cobra.Command, args []string) error {
			before, _ := splitNativeArgs(command, args)
			if len(before) > 1 {
				return errors.New("use accepts at most one agent before --")
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
			cwd, err := resolveCwd(cwdFlag)
			if err != nil {
				return err
			}
			environment, err := xruntime.BuildEnvironment(os.Environ(), cfg, definition.Config, "")
			if err != nil {
				return err
			}
			spec := definition.Interactive(nativeArgs)
			store, err := newStore()
			if err != nil {
				return err
			}
			record := runstore.Record{
				ID: runstore.NewID("use"), Kind: "use", Agent: name, Cwd: cwd,
				StartedAt: time.Now().UTC(), Status: "running",
			}
			ctx, cancel := signalContext(command.Context())
			defer cancel()
			capture := cfg.Recording.Output
			result, runErr := xruntime.RunProcess(ctx, spec, xruntime.ProcessOptions{
				Dir: cwd, Env: environment, Stdin: os.Stdin, Stdout: command.OutOrStdout(),
				Stderr: command.ErrOrStderr(), CaptureStdout: capture,
			})
			record.EndedAt = time.Now().UTC()
			if runErr != nil {
				record.Status = "failed"
				record.ExitCode = 1
				_ = store.Save(record)
				return runErr
			}
			record.ExitCode = result.ExitCode
			record.Status = processStatus(result)
			if capture {
				path, err := store.SaveOutput(record.ID, "output.log", result.Stdout)
				if err != nil {
					return err
				}
				record.OutputFile = path
			}
			if err := store.Save(record); err != nil {
				return err
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

func (a *app) newRunCommand() *cobra.Command {
	var selectedAgent string
	var cwdFlag string
	var asJSON bool
	command := &cobra.Command{
		Use:   "run [agent] <prompt> [-- native-args...]",
		Short: "Run one non-interactive agent task",
		Args: func(command *cobra.Command, args []string) error {
			before, _ := splitNativeArgs(command, args)
			if len(before) == 0 {
				return errors.New("a prompt is required")
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			cfg, _, registry, err := a.load()
			if err != nil {
				return err
			}
			before, nativeArgs := splitNativeArgs(command, args)
			selection := routing.Decision{}
			promptParts := before
			if selectedAgent != "" {
				selection = routing.Decision{Agent: selectedAgent, Source: routing.SourceFlag}
			} else if len(before) > 0 && registry.Has(before[0]) {
				selection = routing.Decision{Agent: before[0], Source: routing.SourcePositional}
				promptParts = before[1:]
			}
			if len(promptParts) == 0 {
				return errors.New("a prompt is required")
			}
			prompt := strings.Join(promptParts, " ")
			if selection.Agent == "" {
				selection, err = routing.Select(cfg, prompt)
				if err != nil {
					return err
				}
			}
			name := selection.Agent
			definition, err := registry.Get(name)
			if err != nil {
				return err
			}
			spec, err := definition.Run(prompt, asJSON, nativeArgs)
			if err != nil {
				return err
			}
			cwd, err := resolveCwd(cwdFlag)
			if err != nil {
				return err
			}
			environment, err := xruntime.BuildEnvironment(os.Environ(), cfg, definition.Config, "")
			if err != nil {
				return err
			}
			store, err := newStore()
			if err != nil {
				return err
			}
			record := runstore.Record{
				ID: runstore.NewID("run"), Kind: "run", Agent: name,
				SelectionSource: selection.Source, RouteRule: selection.Rule, Cwd: cwd,
				StartedAt: time.Now().UTC(), Status: "running",
			}
			ctx, cancel := signalContext(command.Context())
			defer cancel()
			capture := asJSON || cfg.Recording.Output
			var stdout = command.OutOrStdout()
			if asJSON {
				stdout = nil
			}
			processResult, runErr := xruntime.RunProcess(ctx, spec, xruntime.ProcessOptions{
				Dir: cwd, Env: environment, Stdout: stdout, Stderr: command.ErrOrStderr(),
				CaptureStdout: capture, SeparateProcess: asJSON,
			})
			record.EndedAt = time.Now().UTC()
			if runErr != nil {
				record.Status = "failed"
				record.ExitCode = 1
				_ = store.Save(record)
				return runErr
			}
			record.ExitCode = processResult.ExitCode
			record.Status = processStatus(processResult)
			var normalized agent.RunResult
			if asJSON {
				normalized = agent.ParseStructured(definition.Config.Adapter, definition.Config.Output, processResult.Stdout)
				normalized.Agent = name
				normalized.ExitCode = processResult.ExitCode
				normalized.Status = record.Status
				record.SessionID = normalized.SessionID
				if err := encodeJSON(command.OutOrStdout(), normalized); err != nil {
					return err
				}
			}
			if cfg.Recording.Output {
				path, err := store.SaveOutput(record.ID, "output.log", processResult.Stdout)
				if err != nil {
					return err
				}
				record.OutputFile = path
			}
			if err := store.Save(record); err != nil {
				return err
			}
			if processResult.ExitCode != 0 {
				return &ExitError{Code: processResult.ExitCode}
			}
			return nil
		},
	}
	command.Flags().StringVar(&selectedAgent, "agent", "", "agent to run (overrides positional and default selection)")
	command.Flags().StringVar(&cwdFlag, "cwd", "", "working directory")
	command.Flags().BoolVar(&asJSON, "json", false, "return a normalized JSON result")
	return command
}

func processStatus(result xruntime.ProcessResult) string {
	if result.TimedOut {
		return "timed_out"
	}
	if result.Canceled {
		return "canceled"
	}
	if result.ExitCode == 0 {
		return "success"
	}
	return "failed"
}

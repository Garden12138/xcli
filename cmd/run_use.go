package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
			raw, err := openRawOutput(store, record.ID, cfg.Recording.Output)
			if err != nil {
				return err
			}
			var rawWriter io.Writer
			if raw != nil {
				rawWriter = raw
			}
			if err := store.Save(record); err != nil {
				if raw != nil {
					raw.Close()
				}
				return err
			}
			result, runErr := xruntime.RunProcess(ctx, spec, xruntime.ProcessOptions{Dir: cwd, Env: environment, Stdin: os.Stdin, Stdout: command.OutOrStdout(), Stderr: command.ErrOrStderr(), RawOutput: rawWriter})
			normalized, code := agent.Normalize(agent.RunResult{ParseStatus: "not_applicable"}, result.ExitCode, result.Canceled, result.TimedOut, runErr)
			if result.IOError && code == 0 {
				normalized.Status = "failed"
				normalized.ErrorKind = "process_failed"
				normalized.Error = "native process I/O failed"
				code = 1
			}
			record.EndedAt = time.Now().UTC()
			applyResult(&record, normalized)
			if raw != nil {
				if err := raw.Close(); err != nil {
					return err
				}
				record.OutputFile = raw.Name()
			}
			if err := store.Save(record); err != nil {
				return err
			}
			if runErr != nil {
				return runErr
			}
			if code != 0 {
				return &ExitError{Code: code}
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
	var detach bool
	var events bool
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
			if events && (asJSON || detach) {
				return errors.New("--events cannot be combined with --json or --detach")
			}
			cfg, configPath, registry, err := a.load()
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
			structured := asJSON || definition.Config.Adapter != "generic"
			spec, err := definition.Run(prompt, structured, nativeArgs)
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
			if detach {
				if _, err := exec.LookPath(spec.Command); err != nil {
					return fmt.Errorf("agent command %q is not available: %w", spec.Command, err)
				}
				view, err := startDetachedRun(store, detachedRunOptions{
					ConfigPath: configPath, Agent: name, Prompt: prompt, NativeArgs: nativeArgs,
					Cwd: cwd, Structured: structured, RecordOutput: cfg.Recording.Output, Record: record,
				})
				if err != nil {
					return err
				}
				if asJSON {
					return encodeJSON(command.OutOrStdout(), view)
				}
				_, err = fmt.Fprintf(command.OutOrStdout(), "Started job %s (pid %d)\n", view.ID, view.PID)
				return err
			}
			ctx, cancel := signalContext(command.Context())
			defer cancel()
			raw, err := openRawOutput(store, record.ID, cfg.Recording.Output)
			if err != nil {
				return err
			}
			if err := store.Save(record); err != nil {
				if raw != nil {
					raw.Close()
				}
				return err
			}
			stderr := &synchronizedWriter{writer: command.ErrOrStderr()}
			var activity io.Writer = stderr
			if events {
				activity = command.OutOrStdout()
			}
			outcome, runErr := executeNonInteractive(ctx, definition, spec, cwd, environment, stderr,
				record.ID, raw, eventSink(activity, events))
			record.EndedAt = time.Now().UTC()
			applyResult(&record, outcome.Result)
			if raw != nil {
				if err := raw.Close(); err != nil {
					return err
				}
				record.OutputFile = raw.Name()
			}
			if err := store.Save(record); err != nil {
				return err
			}
			if !events {
				if asJSON {
					if err := encodeJSON(command.OutOrStdout(), outcome.Result); err != nil {
						return err
					}
				} else if outcome.Result.Output != "" {
					if _, err := fmt.Fprintln(command.OutOrStdout(), outcome.Result.Output); err != nil {
						return err
					}
				}
			}
			if runErr != nil {
				return runErr
			}
			if outcome.CLIExitCode != 0 {
				return &ExitError{Code: outcome.CLIExitCode}
			}
			return nil

		},
	}
	command.Flags().StringVar(&selectedAgent, "agent", "", "agent to run (overrides positional and default selection)")
	command.Flags().StringVar(&cwdFlag, "cwd", "", "working directory")
	command.Flags().BoolVar(&asJSON, "json", false, "return a normalized JSON result")
	command.Flags().BoolVar(&detach, "detach", false, "run in the background")
	command.Flags().BoolVar(&events, "events", false, "stream versioned JSONL events")
	return command
}

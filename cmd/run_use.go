package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/routing"
	"github.com/Garden12138/xcli/internal/runstore"
	xruntime "github.com/Garden12138/xcli/internal/runtime"
	"github.com/Garden12138/xcli/internal/worktree"
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
				ID: runstore.NewID("use"), Kind: "use", Agent: name,
				AgentVersion: definition.Detect(command.Context()).Version, Cwd: cwd,
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
	var detach bool
	var isolatedWorktree bool
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
			installation := definition.Detect(command.Context())
			record := runstore.Record{
				ID: runstore.NewID("run"), Kind: "run", Agent: name,
				AgentVersion:    installation.Version,
				SelectionSource: selection.Source, RouteRule: selection.Rule, Cwd: cwd,
				StartedAt: time.Now().UTC(), Status: "running",
			}
			if isolatedWorktree {
				if _, err := exec.LookPath(spec.Command); err != nil {
					return fmt.Errorf("agent command %q is not available: %w", spec.Command, err)
				}
				manager, err := worktree.New()
				if err != nil {
					return err
				}
				metadata, err := manager.Create(record.ID, cwd)
				if err != nil {
					return err
				}
				cwd, err = manager.MapPath(metadata, cwd)
				if err != nil {
					return err
				}
				record.Cwd = cwd
				record.WorkspaceMode = "worktree"
				record.Repository = metadata.Repository
				record.WorktreePath = metadata.Path
				record.WorktreeBranch = metadata.Branch
				record.BaseCommit = metadata.BaseCommit
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
			if isolatedWorktree {
				if err := store.Save(record); err != nil {
					return err
				}
			}
			ctx, cancel := signalContext(command.Context())
			defer cancel()
			capture := structured || cfg.Recording.Output
			var stdout = command.OutOrStdout()
			if structured {
				stdout = nil
			}
			outcome, runErr := executeNonInteractive(
				ctx, definition, name, spec, cwd, environment, structured,
				stdout, command.ErrOrStderr(), capture, asJSON,
			)
			record.EndedAt = time.Now().UTC()
			if runErr != nil {
				record.Status = "failed"
				record.ExitCode = 1
				_ = store.Save(record)
				return runErr
			}
			processResult := outcome.Process
			record.ExitCode = processResult.ExitCode
			record.Status = processStatus(processResult)
			if structured {
				normalized := outcome.Result
				record.SessionID = normalized.SessionID
				record.Usage = normalized.Usage
				if asJSON {
					if err := encodeJSON(command.OutOrStdout(), normalized); err != nil {
						return err
					}
				} else if normalized.Output != "" {
					if _, err := fmt.Fprintln(command.OutOrStdout(), normalized.Output); err != nil {
						return err
					}
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
	command.Flags().BoolVar(&detach, "detach", false, "run in the background")
	command.Flags().BoolVar(&isolatedWorktree, "worktree", false, "run in an isolated managed Git worktree")
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

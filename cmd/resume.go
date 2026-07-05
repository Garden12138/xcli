package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/routing"
	"github.com/Garden12138/xcli/internal/runstore"
	xruntime "github.com/Garden12138/xcli/internal/runtime"
	"github.com/spf13/cobra"
)

type resumeTarget struct {
	Agent       string
	SessionID   string
	Cwd         string
	ResumedFrom string
	ResumedStep string
}

func (a *app) newResumeCommand() *cobra.Command {
	var selectedAgent string
	var stepID string
	var cwdFlag string
	var asJSON bool
	command := &cobra.Command{
		Use:   "resume <run-id|session-id> [prompt...] [-- native-args...]",
		Short: "Resume a recorded or native agent session",
		Args: func(command *cobra.Command, args []string) error {
			before, _ := splitNativeArgs(command, args)
			if len(before) == 0 {
				return errors.New("a run id or native session id is required")
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			cfg, _, registry, err := a.load()
			if err != nil {
				return err
			}
			before, nativeArgs := splitNativeArgs(command, args)
			prompt := strings.Join(before[1:], " ")
			interactive := prompt == ""
			if interactive && asJSON {
				return errors.New("--json requires a prompt")
			}

			store, err := newStore()
			if err != nil {
				return err
			}
			target, err := resolveResumeTarget(store, before[0], selectedAgent, stepID)
			if err != nil {
				return err
			}
			definition, err := registry.Get(target.Agent)
			if err != nil {
				return fmt.Errorf("cannot resume session: %w", err)
			}
			spec, err := definition.Resume(target.SessionID, prompt, !interactive, nativeArgs)
			if err != nil {
				return err
			}

			cwdValue := cwdFlag
			if cwdValue == "" {
				cwdValue = target.Cwd
			}
			cwd, err := resolveCwd(cwdValue)
			if err != nil {
				if target.Cwd != "" && cwdFlag == "" {
					return fmt.Errorf("recorded working directory is unavailable; pass --cwd to override it: %w", err)
				}
				return err
			}
			environment, err := xruntime.BuildEnvironment(os.Environ(), cfg, definition.Config, "")
			if err != nil {
				return err
			}

			kind := "run"
			prefix := "run"
			selectionSource := routing.SourceResume
			if interactive {
				kind = "use"
				prefix = "use"
				selectionSource = ""
			}
			record := runstore.Record{
				ID: runstore.NewID(prefix), Kind: kind, Agent: target.Agent,
				SelectionSource: selectionSource, Cwd: cwd, StartedAt: time.Now().UTC(), Status: "running",
				SessionID: target.SessionID, ResumedFrom: target.ResumedFrom, ResumedStep: target.ResumedStep,
			}

			ctx, cancel := signalContext(command.Context())
			defer cancel()
			capture := !interactive || cfg.Recording.Output
			stdout := command.OutOrStdout()
			if !interactive {
				stdout = nil
			}
			processResult, runErr := xruntime.RunProcess(ctx, spec, xruntime.ProcessOptions{
				Dir: cwd, Env: environment, Stdin: interactiveReader(interactive),
				Stdout: stdout, Stderr: command.ErrOrStderr(), CaptureStdout: capture,
				SeparateProcess: !interactive && asJSON,
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

			if !interactive {
				normalized := agent.ParseStructured(definition.Config.Adapter, definition.Config.Output, processResult.Stdout)
				normalized.Agent = target.Agent
				if normalized.SessionID == "" {
					normalized.SessionID = target.SessionID
				}
				normalized.ExitCode = processResult.ExitCode
				normalized.Status = record.Status
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
	command.Flags().StringVar(&selectedAgent, "agent", "", "agent for a native session id")
	command.Flags().StringVar(&stepID, "step", "", "workflow step to resume")
	command.Flags().StringVar(&cwdFlag, "cwd", "", "working directory override")
	command.Flags().BoolVar(&asJSON, "json", false, "return a normalized JSON result")
	return command
}

func interactiveReader(interactive bool) *os.File {
	if interactive {
		return os.Stdin
	}
	return nil
}

func resolveResumeTarget(store *runstore.Store, value, selectedAgent, stepID string) (resumeTarget, error) {
	record, err := store.Load(value)
	if err == nil {
		return resumeTargetFromRecord(record, selectedAgent, stepID)
	}
	if !os.IsNotExist(err) {
		return resumeTarget{}, fmt.Errorf("load run record %q: %w", value, err)
	}
	if stepID != "" {
		return resumeTarget{}, errors.New("--step requires an existing workflow run record")
	}
	if selectedAgent == "" {
		return resumeTarget{}, fmt.Errorf("run record %q was not found; pass --agent to use it as a native session id", value)
	}
	return resumeTarget{Agent: selectedAgent, SessionID: value}, nil
}

func resumeTargetFromRecord(record runstore.Record, selectedAgent, stepID string) (resumeTarget, error) {
	target := resumeTarget{Cwd: record.Cwd, ResumedFrom: record.ID}
	if record.Kind == "workflow" {
		if stepID == "" {
			return resumeTarget{}, fmt.Errorf("workflow run %q requires --step", record.ID)
		}
		var found *runstore.StepRecord
		for index := range record.Steps {
			if record.Steps[index].ID == stepID {
				found = &record.Steps[index]
				break
			}
		}
		if found == nil {
			return resumeTarget{}, fmt.Errorf("workflow run %q has no step %q", record.ID, stepID)
		}
		target.Agent = found.Agent
		target.SessionID = found.SessionID
		target.ResumedStep = found.ID
	} else {
		if stepID != "" {
			return resumeTarget{}, fmt.Errorf("--step can only be used with a workflow run record")
		}
		target.Agent = record.Agent
		target.SessionID = record.SessionID
	}
	if target.Agent == "" {
		return resumeTarget{}, fmt.Errorf("run record %q does not identify an agent", record.ID)
	}
	if selectedAgent != "" && selectedAgent != target.Agent {
		return resumeTarget{}, fmt.Errorf("run record %q uses agent %q, not %q", record.ID, target.Agent, selectedAgent)
	}
	if target.SessionID == "" {
		if target.ResumedStep != "" {
			return resumeTarget{}, fmt.Errorf("workflow step %q has no recorded session id", target.ResumedStep)
		}
		return resumeTarget{}, fmt.Errorf("run record %q has no recorded session id", record.ID)
	}
	return target, nil
}

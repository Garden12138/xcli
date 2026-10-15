package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/worktree"
	"io"
	"time"

	"github.com/Garden12138/xcli/internal/runstore"
	"github.com/Garden12138/xcli/internal/workflow"
	"github.com/spf13/cobra"
)

func (a *app) newWorkflowCommand() *cobra.Command {
	command := &cobra.Command{Use: "workflow", Short: "Validate and run agent workflows"}
	command.AddCommand(a.newWorkflowValidateCommand(), a.newWorkflowRunCommand())
	return command
}

func (a *app) newWorkflowValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate a workflow file",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			cfg, _, registry, err := a.load()
			if err != nil {
				return err
			}
			definition, path, err := workflow.Load(args[0])
			if err != nil {
				return err
			}
			networks := make(map[string]struct{}, len(cfg.Networks))
			for name := range cfg.Networks {
				networks[name] = struct{}{}
			}
			if err := workflow.Validate(definition, registry, networks); err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "Workflow is valid: %s\n", path)
			return nil
		},
	}
}

func (a *app) newWorkflowRunCommand() *cobra.Command {
	var values []string
	var recordOutput bool
	var asJSON bool
	var maxParallel int
	var detach bool
	var events bool
	var isolated bool
	command := &cobra.Command{
		Use:   "run <file>",
		Short: "Run a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if events && (asJSON || detach) {
				return fmt.Errorf("--events cannot be combined with --json or --detach")
			}
			cfg, configPath, registry, err := a.load()
			if err != nil {
				return err
			}
			definition, path, err := workflow.Load(args[0])
			if err != nil {
				return err
			}
			overrides, err := workflow.ParseVars(values)
			if err != nil {
				return err
			}
			store, err := newStore()
			if err != nil {
				return err
			}
			progress := command.ErrOrStderr()
			var sink func(agent.Event) error
			if events {
				progress = io.Discard
				sink = eventSink(command.OutOrStdout(), true)
			} else {
				sink = eventSink(command.ErrOrStderr(), false)
			}
			runner := workflow.Runner{Config: cfg, Registry: registry, Store: store, Progress: progress, Stderr: command.ErrOrStderr(), Events: sink}

			ctx, cancel := signalContext(command.Context())
			defer cancel()
			options := workflow.RunOptions{
				VariableOverrides: overrides, Worktree: isolated,
				RecordOutput: recordOutput || cfg.Recording.Output,
			}
			if command.Flags().Changed("max-parallel") {
				options.MaxParallel = &maxParallel
			}
			prepared, err := runner.Prepare(path, definition, options)
			if err != nil {
				return err
			}
			if detach {
				if plan := runner.BuildPlan(prepared); !plan.Applicable {
					return fmt.Errorf("workflow preflight failed: %+v", plan.Steps)
				}
				if prepared.Workspace != nil {
					manager, err := worktree.New()
					if err != nil {
						return err
					}
					metadata, err := manager.Create(prepared.ID, prepared.SourceCwd)
					if err != nil {
						return err
					}
					if metadata.BaseCommit != prepared.Workspace.BaseCommit {
						return fmt.Errorf("repository baseline changed after preparation")
					}
					for _, cwd := range prepared.StepCwds {
						if err := worktree.CheckDirectory(metadata.Path, cwd); err != nil {
							return err
						}
					}
				}

				record := runstore.Record{
					ID: prepared.ID, Kind: "workflow", Workflow: definition.Name,
					WorkflowFile: path, MaxParallel: prepared.MaxParallel, Cwd: prepared.WorkingDirectory,
					StartedAt: time.Now().UTC(), Status: "running",
				}
				for _, step := range definition.Steps {
					record.Steps = append(record.Steps, runstore.StepRecord{ID: step.ID, Agent: step.Agent, Command: step.Command, Cwd: prepared.StepCwds[step.ID], Status: "pending"})
				}
				if prepared.Workspace != nil {
					m := prepared.Workspace
					record.WorkspaceMode = "worktree"
					record.Repository = m.Repository
					record.WorktreePath = m.Path
					record.WorktreeBranch = m.Branch
					record.BaseCommit = m.BaseCommit
				}
				view, err := startDetachedWorkflow(store, configPath, record, prepared)
				if err != nil {
					return err
				}
				if asJSON {
					return encodeJSON(command.OutOrStdout(), view)
				}
				_, err = fmt.Fprintf(command.OutOrStdout(), "Started workflow job %s (pid %d)\n", view.ID, view.PID)
				return err
			}
			if events {
				if err := json.NewEncoder(command.OutOrStdout()).Encode(agent.Event{Version: 1, RunID: prepared.ID, Time: time.Now().UTC(), Type: "started"}); err != nil {
					return err
				}
			}
			execution, err := runner.RunPrepared(ctx, prepared, nil)
			if err != nil {
				return err
			}
			if events {
				kind := "completed"
				if execution.ExitCode != 0 {
					kind = "failed"
				}
				if err := json.NewEncoder(command.OutOrStdout()).Encode(agent.Event{Version: 1, RunID: execution.ID, Time: time.Now().UTC(), Type: kind, Result: execution}); err != nil {
					return err
				}
			} else if asJSON {
				if err := encodeJSON(command.OutOrStdout(), execution); err != nil {
					return err
				}
			} else if err := writeWorkflowExecution(command.OutOrStdout(), execution); err != nil {
				return err
			}
			if execution.ExitCode != 0 {
				return &ExitError{Code: execution.ExitCode}
			}
			return nil
		},
	}
	command.Flags().StringArrayVar(&values, "var", nil, "workflow variable override (key=value)")
	command.Flags().BoolVar(&recordOutput, "record-output", false, "persist raw step output")
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON execution summary")
	command.Flags().IntVar(&maxParallel, "max-parallel", 0, "override maximum parallel workflow steps")
	command.Flags().BoolVar(&detach, "detach", false, "run the workflow in the background")
	command.Flags().BoolVar(&events, "events", false, "stream versioned JSONL events")
	command.Flags().BoolVar(&isolated, "worktree", false, "run in an isolated managed Git worktree")
	return command
}

func writeWorkflowExecution(writer io.Writer, execution workflow.Execution) error {
	if _, err := fmt.Fprintf(
		writer, "Workflow %s: %s (run %s, max parallel %d)\n",
		execution.Name, execution.Status, execution.ID, execution.MaxParallel,
	); err != nil {
		return err
	}
	for _, step := range execution.Steps {
		if _, err := fmt.Fprintf(writer, "- %s: %s", step.ID, step.Status); err != nil {
			return err
		}
		if step.Error != "" {
			if _, err := fmt.Fprintf(writer, " (%s)", step.Error); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(writer); err != nil {
			return err
		}
	}
	return nil
}

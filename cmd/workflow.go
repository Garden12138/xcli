package cmd

import (
	"fmt"
	"io"
	"os/exec"
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
	command := &cobra.Command{
		Use:   "run <file>",
		Short: "Run a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
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
			progress := command.OutOrStdout()
			if asJSON {
				progress = io.Discard
			}
			runner := workflow.Runner{
				Config: cfg, Registry: registry, Store: store,
				Progress: progress, Stderr: command.ErrOrStderr(),
			}
			ctx, cancel := signalContext(command.Context())
			defer cancel()
			options := workflow.RunOptions{
				VariableOverrides: overrides,
				RecordOutput:      recordOutput || cfg.Recording.Output,
			}
			if command.Flags().Changed("max-parallel") {
				options.MaxParallel = &maxParallel
			}
			prepared, err := runner.Prepare(path, definition, options)
			if err != nil {
				return err
			}
			if detach {
				checked := map[string]bool{}
				for _, step := range definition.Steps {
					if checked[step.Agent] {
						continue
					}
					checked[step.Agent] = true
					agentDefinition, err := registry.Get(step.Agent)
					if err != nil {
						return err
					}
					if _, err := exec.LookPath(agentDefinition.Config.Command); err != nil {
						return fmt.Errorf("agent command %q is not available: %w", agentDefinition.Config.Command, err)
					}
				}
				record := runstore.Record{
					ID: runstore.NewID("workflow"), Kind: "workflow", Workflow: definition.Name,
					WorkflowFile: path, MaxParallel: prepared.MaxParallel, Cwd: prepared.WorkingDirectory,
					StartedAt: time.Now().UTC(), Status: "running",
				}
				for _, step := range definition.Steps {
					record.Steps = append(record.Steps, runstore.StepRecord{ID: step.ID, Agent: step.Agent, Status: "pending"})
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
			execution, err := runner.RunPrepared(ctx, prepared, nil)
			if err != nil {
				return err
			}
			if asJSON {
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

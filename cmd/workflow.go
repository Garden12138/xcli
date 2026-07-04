package cmd

import (
	"fmt"
	"io"

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
	command := &cobra.Command{
		Use:   "run <file>",
		Short: "Run a workflow",
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
			execution, err := runner.Run(ctx, path, definition, options)
			if err != nil {
				return err
			}
			if asJSON {
				if err := encodeJSON(command.OutOrStdout(), execution); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(
					command.OutOrStdout(), "Workflow %s: %s (run %s, max parallel %d)\n",
					execution.Name, execution.Status, execution.ID, execution.MaxParallel,
				)
				for _, step := range execution.Steps {
					fmt.Fprintf(command.OutOrStdout(), "- %s: %s", step.ID, step.Status)
					if step.Error != "" {
						fmt.Fprintf(command.OutOrStdout(), " (%s)", step.Error)
					}
					fmt.Fprintln(command.OutOrStdout())
				}
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
	return command
}

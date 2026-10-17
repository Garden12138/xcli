package cmd

import (
	"fmt"
	"github.com/Garden12138/xcli/internal/workflow"
	"io"
)

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

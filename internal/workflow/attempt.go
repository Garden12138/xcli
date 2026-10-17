package workflow

import (
	"context"
	"os"
)

// ExecuteAttempt exposes the existing bounded step runner to Task without a
// YAML file, DAG, or a second process implementation. Task owns its Run record.
// Task inputs are literal: user text is never workflow template syntax.
func (r *Runner) ExecuteAttempt(ctx context.Context, runID, cwd, boundary string, step Step, recordOutput bool) StepResult {
	dir, err := os.MkdirTemp("", "xcli-task-attempt-")
	if err != nil {
		return StepResult{ID: step.ID, Status: "failed", CLIExitCode: 1, Error: err.Error()}
	}
	defer os.RemoveAll(dir)
	copyRunner := *r
	copyRunner.workspaceRoot = boundary
	copyRunner.literal = true
	result := copyRunner.runStep(ctx, cwd, dir, step, templateContext{}, recordOutput, runID)
	if !recordOutput {
		result.OutputFile = ""
	}
	return result
}

func (r *Runner) render(value string, data templateContext) (string, error) {
	if r.literal {
		return value, nil
	}
	return resolveTemplate(value, data)
}

package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/proc"
)

func RunVerify(ctx context.Context, dir string, cmds []string, timeout time.Duration, logDir string, round int) []api.VerifyResult {
	results := []api.VerifyResult{}
	for i, command := range cmds {
		logPath := filepath.Join(logDir, fmt.Sprintf("verify-%d.log", i+1))
		out := proc.Run(ctx, proc.Spec{Path: "/bin/sh", Args: []string{"-c", "exec 2>&1\n" + command}, Dir: dir}, proc.Options{Timeout: timeout, StdoutFile: logPath, RedactProxy: true})
		code := out.ExitCode
		if out.TimedOut {
			code = 124
		}
		if out.StartErr != nil {
			_ = os.WriteFile(logPath, []byte("请检查验证命令、工作目录和日志目录："+out.StartErr.Error()), 0600)
		}
		results = append(results, api.VerifyResult{Command: command, Round: round, ExitCode: code, Passed: code == 0 && out.StartErr == nil && !out.Cancelled && !out.TimedOut, LogFile: logPath})
		if ctx.Err() != nil {
			break
		}
	}
	return results
}

func Apply(patchPath, repo string) error {
	raw, err := os.ReadFile(patchPath)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	_, err = gitOutput(repo, "apply", "--3way", patchPath)
	return err
}

func RemoveWorktree(repo, path, branch string) error {
	if _, err := gitOutput(repo, "worktree", "remove", "--force", path); err != nil {
		return err
	}
	if branch != "" {
		_, err := gitOutput(repo, "branch", "-D", branch)
		return err
	}
	return nil
}

//go:build unix

package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/store"
	"github.com/Garden12138/xcli/internal/workspace"
)

func (e *Engine) Detach(t *store.Task) (err error) {
	started := false
	defer func() {
		if err == nil || started {
			return
		}
		rec := t.Snapshot()
		now := time.Now().UTC()
		result := api.Result{Version: api.Version, TaskID: t.ID(), Capability: rec.Request.Capability, Status: api.StatusFailed, Summary: err.Error(), Artifacts: []api.Artifact{}, Steps: []api.StepResult{}, Metadata: rec.Request.Metadata, StartedAt: now, FinishedAt: now, Error: &api.Error{Kind: "failed", Message: err.Error()}}
		if saveErr := t.Update(func(r *store.Record) { r.Status = api.StatusFailed; r.Result = &result }); saveErr != nil {
			err = errors.Join(err, saveErr)
			return
		}
		_, saveErr := t.Emit("task.finished", "", "", result)
		err = errors.Join(err, saveErr)
	}()
	self := os.Getenv("XCLI_SELF")
	if self == "" {
		var err error
		self, err = os.Executable()
		if err != nil {
			return err
		}
	}
	log, err := os.OpenFile(t.Path("worker.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	input, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer input.Close()
	cmd := exec.Command(self, "--home", e.Home, "__worker", t.ID())
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = input, log, log
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("请检查 xcli 执行文件和 worker.log 后重新提交：%w", err)
	}
	started = true
	return cmd.Process.Release()
}

func (e *Engine) Status(id string) (store.Record, error) {
	t, err := e.Store.Load(id)
	if err != nil {
		return store.Record{}, err
	}
	rec := t.Snapshot()
	if rec.Status == api.StatusRunning && (rec.PID <= 0 || errors.Is(syscall.Kill(rec.PID, 0), syscall.ESRCH)) {
		now := time.Now().UTC()
		result := api.Result{Version: api.Version, TaskID: id, Status: api.StatusFailed, Capability: rec.Request.Capability, Summary: "任务进程已结束，请查看 worker.log 并重新提交任务", Plan: rec.Plan, Steps: rec.Steps, Artifacts: []api.Artifact{}, Metadata: rec.Request.Metadata, StartedAt: rec.CreatedAt, FinishedAt: now, DurationMS: now.Sub(rec.CreatedAt).Milliseconds(), Error: &api.Error{Kind: "worker_lost", Message: "任务进程已结束，请查看 worker.log 并重新提交任务"}}
		if err := t.Update(func(r *store.Record) { r.Status = api.StatusFailed; r.Result = &result; r.PID = 0 }); err != nil {
			return rec, err
		}
		_, err = t.Emit("task.finished", "", "", result)
		if err != nil {
			return t.Snapshot(), err
		}
		rec = t.Snapshot()
	}
	return rec, nil
}

func (e *Engine) Wait(ctx context.Context, id string) (api.Result, error) {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		rec, err := e.Status(id)
		if err != nil {
			return api.Result{}, err
		}
		if rec.Status.Finished() {
			if rec.Result == nil {
				return api.Result{}, errors.New("请查看 task.json 中的结果或重新提交任务")
			}
			return *rec.Result, nil
		}
		select {
		case <-ctx.Done():
			return api.Result{}, ctx.Err()
		case <-tick.C:
		}
	}
}

func (e *Engine) Cancel(id string) error {
	rec, err := e.Status(id)
	if err != nil {
		return err
	}
	if rec.Status.Finished() {
		return errors.New("任务已结束，请用 xcli result 查看结果")
	}
	if rec.PID <= 0 {
		return errors.New("任务正在启动，请稍后运行 xcli cancel 重试")
	}
	if err := syscall.Kill(rec.PID, syscall.SIGTERM); err != nil {
		return fmt.Errorf("请用 xcli status 检查任务进程后重试：%w", err)
	}
	return nil
}

func (e *Engine) Follow(ctx context.Context, id string, after int, fn func(api.Event) error) error {
	t, err := e.Store.Load(id)
	if err != nil {
		return err
	}
	return store.Follow(ctx, t.Dir(), after, fn)
}

func (e *Engine) Resume(parentID, instruction string) (api.Request, error) {
	if strings.TrimSpace(instruction) == "" {
		return api.Request{}, errors.New("请提供追加要求")
	}
	rec, err := e.Status(parentID)
	if err != nil {
		return api.Request{}, err
	}
	if !rec.Status.Finished() {
		return api.Request{}, errors.New("请等待父任务结束后再续接")
	}
	t, _ := e.Store.Load(parentID)
	req := rec.Request
	req.ParentTask = parentID
	req.Goal = rec.Request.Goal + "\n追加要求：" + instruction
	req.Plan = nil
	req.PlanOnly = false
	req.Isolate = false
	var step api.StepResult
	step = lastExecuted(rec.Steps)
	if step.Agent == "" {
		return api.Request{}, errors.New("请重新提交任务，父任务还没有已执行的步骤")
	}
	req.Capability, req.Agent = api.Capability(step.Kind), step.Agent
	if rec.Result != nil && rec.Result.Changes != nil {
		req.Workdir = rec.Result.Changes.Workdir
	} else if rec.Request.Isolate {
		req.Workdir = t.Path("worktree")
	} else if req.Workdir == "" {
		req.Workdir = t.Path("work")
	}
	if req.OutputDir == "" {
		req.OutputDir = t.Path("out", step.ID)
	}
	if step.Kind == api.KindImage && rec.Result != nil {
		req.Images = []string{}
		for _, a := range rec.Result.Artifacts {
			req.Images = append(req.Images, a.Path)
		}
	}
	return req, nil
}

func lastExecuted(steps []api.StepResult) api.StepResult {
	for i := len(steps) - 1; i >= 0; i-- {
		s := steps[i]
		if s.Rounds > 0 && s.Agent != "" && s.Agent != "auto" && s.Status != api.StatusSkipped {
			return s
		}
	}
	return api.StepResult{}
}

func (e *Engine) Apply(id string, removeWorktree bool) error {
	rec, err := e.Status(id)
	if err != nil {
		return err
	}
	if !rec.Status.Finished() || rec.Result == nil || rec.Result.Changes == nil || rec.Result.Changes.Isolation != "worktree" {
		return errors.New("请先完成一个带 --isolate 的代码任务，再运行 xcli apply")
	}
	c := rec.Result.Changes
	if err := workspace.Apply(c.DiffFile, rec.Request.Workdir); err != nil {
		return err
	}
	if removeWorktree {
		return workspace.RemoveWorktree(rec.Request.Workdir, c.Workdir, c.Branch)
	}
	return nil
}

func (e *Engine) GC(olderThan time.Duration) (int, error) {
	if olderThan <= 0 {
		return 0, errors.New("请将 older-than 设为正时长，例如 7d")
	}
	rows, err := e.Store.List(0)
	if err != nil {
		return 0, err
	}
	removed := 0
	cutoff := time.Now().Add(-olderThan)
	for _, r := range rows {
		if !r.Status.Finished() {
			continue
		}
		ended := r.UpdatedAt
		if r.Result != nil && !r.Result.FinishedAt.IsZero() {
			ended = r.Result.FinishedAt
		}
		if !ended.Before(cutoff) {
			continue
		}
		if r.Request.Isolate && !r.Request.PlanOnly && r.Request.Workdir != "" {
			path := filepath.Join(e.Store.Root, r.ID, "worktree")
			if _, err := os.Stat(path); err == nil {
				if err := workspace.RemoveWorktree(r.Request.Workdir, path, "xcli/"+r.ID); err != nil {
					return removed, err
				}
			}
		}
		if err := e.Store.Remove(r.ID); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

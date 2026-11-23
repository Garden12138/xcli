package engine

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/driver"
	"github.com/Garden12138/xcli/internal/health"
	"github.com/Garden12138/xcli/internal/orchestrator"
	"github.com/Garden12138/xcli/internal/proc"
	"github.com/Garden12138/xcli/internal/store"
	"github.com/Garden12138/xcli/internal/workspace"
)

type Engine struct {
	Home       string
	Config     config.Config
	Store      *store.Store
	Health     *health.Cache
	Drivers    map[string]driver.Driver
	PathList   string
	ShellProxy map[string]string
}

func New(home string) (*Engine, error) {
	home, err := filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(home)
	if err != nil {
		return nil, err
	}
	st, err := store.Open(home)
	if err != nil {
		return nil, err
	}
	path, proxy := proc.SearchEnvironment(cfg.Path, filepath.Join(home, "cache", "env.json"), cfg.Defaults.InheritShellProxy)
	return &Engine{Home: home, Config: cfg, Store: st, Health: health.Open(home), Drivers: driver.All(cfg), PathList: path, ShellProxy: proxy}, nil
}

func (e *Engine) Create(req api.Request) (*store.Task, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	req, err = e.Normalize(req, cwd)
	if err != nil {
		return nil, err
	}
	return e.Store.Create(req)
}

func (e *Engine) Run(ctx context.Context, t *store.Task, sink func(api.Event)) (result api.Result) {
	req := t.Snapshot().Request
	env := &orchestrator.Env{Config: e.Config, Drivers: e.Drivers, Health: e.Health, Task: t, Req: req, PathList: e.PathList, ShellProxy: e.ShellProxy, LockDir: filepath.Join(e.Home, "locks"), Now: time.Now, Sink: sink}
	started := time.Now().UTC()
	result = api.Result{Version: api.Version, Status: api.StatusFailed, Artifacts: []api.Artifact{}, Steps: []api.StepResult{}}
	emit := func(typ string, data any) {
		if ev, err := t.Emit(typ, "", "", data); err == nil && sink != nil {
			sink(ev)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(req.TimeoutSec)*time.Second)
	defer cancel()
	defer func() {
		if ctx.Err() != nil {
			kind, message := "timeout", "任务已超时，请增加 timeout 后重试"
			if ctx.Err() == context.Canceled {
				result.Status = api.StatusCancelled
				kind, message = "cancelled", "任务已取消，可重新提交任务继续"
			} else {
				result.Status = api.StatusFailed
			}
			result.Error = &api.Error{Kind: kind, Message: message}
			if result.Summary == "" {
				result.Summary = message
			}
		}
		result.TaskID, result.Capability, result.Metadata = t.ID(), req.Capability, req.Metadata
		result.StartedAt, result.FinishedAt = started, time.Now().UTC()
		result.DurationMS = result.FinishedAt.Sub(started).Milliseconds()
		if err := t.Update(func(r *store.Record) {
			r.Status = result.Status
			r.Result = &result
			r.Plan = result.Plan
			r.Steps = result.Steps
			r.PID = 0
		}); err != nil {
			result.Status = api.StatusFailed
			result.Error = &api.Error{Kind: "failed", Message: "请检查任务目录写入权限：" + err.Error()}
		}
		emit("task.finished", result)
		e.appendUsage(t.Dir(), store.Record{ID: t.ID(), Status: result.Status, Request: req, Result: &result}, env.Calls())
	}()
	if err := t.Update(func(r *store.Record) { r.Status = api.StatusRunning; r.PID = os.Getpid() }); err != nil {
		result.Error = &api.Error{Kind: "failed", Message: "请检查任务目录写入权限：" + err.Error()}
		return result
	}
	emit("task.started", map[string]any{"capability": req.Capability, "goal": req.Goal})
	prepared := req
	if req.PlanOnly {
		prepared.Isolate = false
	}
	ws, notices, err := workspace.Prepare(t.Dir(), t.ID(), prepared)
	if err != nil {
		result.Error = &api.Error{Kind: "failed", Message: err.Error()}
		result.Summary = err.Error()
		return result
	}
	for _, notice := range notices {
		emit("notice", map[string]string{"text": notice})
	}
	env.WS = ws
	if req.ParentTask != "" {
		if parent, err := e.Store.Load(req.ParentTask); err == nil {
			step := lastExecuted(parent.Snapshot().Steps)
			if step.Agent == req.Agent && api.Capability(step.Kind) == req.Capability {
				env.ResumeSession = step.SessionID
			}
		}
	}
	plan := orchestrator.RulesPlan(req, e.Config)
	if req.Plan != nil {
		plan = *req.Plan
		plan.Source, plan.Planner = "explicit", ""
	} else if req.Capability == api.CapAuto {
		plan = env.Plan(ctx)
	}
	if err := orchestrator.ValidatePlan(plan, req, e.Config.EnabledAgents(), e.Config.Defaults.MaxSteps); err != nil {
		result.Error = &api.Error{Kind: "invalid_request", Message: err.Error()}
		result.Summary = err.Error()
		return result
	}
	result.Plan = &plan
	if issue := orchestrator.NewRouter(env).CheckPlanAvailability(plan); issue != nil {
		result.Error, result.Summary = issue, issue.Message
		return result
	}
	_ = t.Update(func(r *store.Record) { r.Plan = &plan })
	emit("plan.ready", map[string]any{"plan": plan})
	if req.PlanOnly {
		result.Status = api.StatusPlanned
		result.Summary = plan.Reasoning
		return result
	}
	return orchestrator.Execute(ctx, env, plan)
}

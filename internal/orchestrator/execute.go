package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/driver"
	"github.com/Garden12138/xcli/internal/recipe"
	"github.com/Garden12138/xcli/internal/store"
)

func (e *Env) runFallback(ctx context.Context, s api.Step, up []recipe.Handoff) (out StepOutcome) {
	start := e.now()
	defer func() {
		out.Result.DurationMS = e.now().Sub(start).Milliseconds()
		_ = e.Task.Update(func(r *store.Record) {
			for i := range r.Steps {
				if r.Steps[i].ID == s.ID {
					r.Steps[i] = out.Result
					return
				}
			}
		})
	}()
	agent := s.Agent
	if e.Req.Agent != "" {
		agent = e.Req.Agent
	}
	if agent == "" || agent == "auto" {
		var issue *api.Error
		agent, issue = NewRouter(e).Pick(s.Kind)
		if issue != nil {
			issue.StepID = s.ID
			e.emit("step.finished", s.ID, "", map[string]any{"status": api.StatusFailed, "summary": issue.Message})
			return StepOutcome{Result: api.StepResult{ID: s.ID, Kind: s.Kind, Status: api.StatusFailed, Fallbacks: []string{}, Error: issue}}
		}
	}
	fallback := e.Config.Defaults.Fallback
	if e.Req.Fallback != nil {
		fallback = *e.Req.Fallback
	}
	tried, records := []string{}, []string{}
	for attempt := 0; attempt < 3; attempt++ {
		runner := *e
		if attempt > 0 {
			runner.logSubdir = filepath.Join(e.logSubdir, fmt.Sprintf("fallback-%d-%s", attempt, agent))
		}
		out = runner.RunStep(ctx, s, agent, up)
		out.Result.Fallbacks = append([]string{}, records...)
		if out.Result.Error == nil || !fallback || e.Req.Agent != "" || attempt == 2 || !driver.ErrorKind(out.Result.Error.Kind).Switchable(s.Kind) {
			break
		}
		tried = append(tried, agent)
		next, err := NewRouter(e).Pick(s.Kind, tried...)
		if err != nil {
			break
		}
		records = append(records, agent+": "+out.Result.Error.Kind)
		e.emit("agent.fallback", s.ID, agent, map[string]string{"from": agent, "to": next, "reason": out.Result.Error.Kind})
		agent = next
	}
	return out
}

func Execute(ctx context.Context, e *Env, plan api.Plan) api.Result {
	e.state()
	states := make([]api.Status, len(plan.Steps))
	outcomes := make([]StepOutcome, len(plan.Steps))
	index := map[string]int{}
	pending := []api.StepResult{}
	for i, s := range plan.Steps {
		states[i] = api.StatusPending
		index[s.ID] = i
		pending = append(pending, api.StepResult{ID: s.ID, Kind: s.Kind, Agent: s.Agent, Status: api.StatusPending, Fallbacks: []string{}})
	}
	_ = e.Task.Update(func(r *store.Record) { r.Plan = &plan; r.Steps = pending })
	type completed struct {
		index int
		out   StepOutcome
	}
	done := make(chan completed, len(plan.Steps))
	codeLocks := map[string]*sync.Mutex{e.WS.Dir: {}}
	running, finished := 0, 0
	for finished < len(plan.Steps) {
		before := finished
		for i, s := range plan.Steps {
			if states[i] != api.StatusPending {
				continue
			}
			ready, blocked := true, false
			for _, dependency := range s.DependsOn {
				state := states[index[dependency]]
				if state == api.StatusFailed || state == api.StatusSkipped || state == api.StatusCancelled {
					blocked = true
				}
				if state != api.StatusCompleted && state != api.StatusNeedsAttention {
					ready = false
				}
			}
			if blocked || ctx.Err() != nil {
				status := api.StatusSkipped
				if ctx.Err() != nil {
					status = api.StatusCancelled
				}
				states[i] = status
				finished++
				outcomes[i].Result = api.StepResult{ID: s.ID, Kind: s.Kind, Agent: s.Agent, Status: status, Fallbacks: []string{}}
				e.status(s.ID, s.Agent, 0, status)
				e.emit("step.finished", s.ID, s.Agent, map[string]any{"status": status, "summary": "依赖未完成或任务已取消", "error": nil})
				continue
			}
			if !ready || running >= max(1, e.Config.Defaults.MaxParallel) {
				continue
			}
			up := []recipe.Handoff{}
			for _, dependency := range s.DependsOn {
				o := outcomes[index[dependency]]
				paths := []string{}
				for _, a := range o.Artifacts {
					paths = append(paths, a.Path)
				}
				up = append(up, recipe.Handoff{StepID: dependency, Agent: o.Result.Agent, Kind: o.Result.Kind, Summary: o.Text, Artifacts: paths, Files: o.Files})
			}
			states[i] = api.StatusRunning
			running++
			go func() {
				if s.Kind == api.KindCode {
					lock := codeLocks[e.WS.Dir]
					if !lock.TryLock() {
						e.status(s.ID, s.Agent, 0, api.StatusWaiting)
						e.emit("step.waiting", s.ID, s.Agent, map[string]string{"reason": "等待工作区写入"})
						lock.Lock()
					}
					defer lock.Unlock()
				}
				runner := *e
				if i != 0 {
					runner.ResumeSession = ""
				}
				done <- completed{i, runner.runFallback(ctx, s, up)}
			}()
		}
		if running == 0 {
			if finished < len(plan.Steps) {
				if finished > before {
					continue
				}
				return api.Result{Version: api.Version, Status: api.StatusFailed, Error: &api.Error{Kind: "invalid_request", Message: "请检查计划的循环依赖与步骤 ID"}}
			}
			break
		}
		result := <-done
		running--
		finished++
		outcomes[result.index] = result.out
		states[result.index] = result.out.Result.Status
	}
	result := api.Result{Version: api.Version, Status: api.StatusCompleted, Plan: &plan, Artifacts: []api.Artifact{}, Steps: []api.StepResult{}}
	downstream := map[string]bool{}
	for _, s := range plan.Steps {
		for _, id := range s.DependsOn {
			downstream[id] = true
		}
	}
	summaries := []string{}
	for i, s := range plan.Steps {
		out := outcomes[i]
		result.Steps = append(result.Steps, out.Result)
		result.Artifacts = append(result.Artifacts, out.Artifacts...)
		result.Verify = append(result.Verify, out.Verify...)
		if out.Changes != nil {
			result.Changes = out.Changes
		}
		if out.Review != nil {
			result.Review = out.Review
		}
		if s.Kind == api.KindAsk && out.JSON != nil {
			result.Output = out.JSON
		}
		if !downstream[s.ID] && out.Text != "" {
			summaries = append(summaries, out.Text)
		}
		if out.Result.Status == api.StatusFailed {
			result.Status = api.StatusFailed
			if result.Error == nil {
				result.Error = out.Result.Error
			}
		} else if out.Result.Status == api.StatusNeedsAttention && result.Status == api.StatusCompleted {
			result.Status = api.StatusNeedsAttention
		}
	}
	for i, s := range plan.Steps {
		if !downstream[s.ID] && s.Kind == api.KindAsk && outcomes[i].JSON != nil {
			result.Output = outcomes[i].JSON
		}
	}
	result.Summary = strings.Join(summaries, "\n\n")
	if ctx.Err() != nil {
		result.Status = api.StatusFailed
		kind := "timeout"
		if ctx.Err() == context.Canceled {
			result.Status = api.StatusCancelled
			kind = "cancelled"
		}
		result.Error = &api.Error{Kind: kind, Message: "任务已结束，请检查任务结果后重新提交需要继续的步骤"}
	}
	if result.Summary == "" && result.Error != nil {
		result.Summary = result.Error.Message
	}
	return result
}

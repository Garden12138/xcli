package orchestrator

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/driver"
	"github.com/Garden12138/xcli/internal/proc"
)

type Router struct{ env *Env }

func NewRouter(e *Env) *Router { return &Router{env: e} }

func (r *Router) candidateNames(kind api.Kind, exclude []string) []string {
	e := r.env
	allowed := e.Req.Agents
	if len(allowed) == 0 {
		allowed = config.AgentOrder
	}
	if e.Req.Agent != "" {
		allowed = []string{e.Req.Agent}
	}
	choices := []string{}
	for _, name := range allowed {
		if !e.Config.Enabled(name) || slices.Contains(choices, name) || slices.Contains(exclude, name) {
			continue
		}
		if kind == "" || e.Config.Strength(name, string(kind)) > 0 {
			choices = append(choices, name)
		}
	}
	if kind != "" {
		sort.SliceStable(choices, func(i, j int) bool {
			return e.Config.Strength(choices[i], string(kind)) > e.Config.Strength(choices[j], string(kind))
		})
	}
	return choices
}

func (r *Router) Candidates(kind api.Kind, exclude ...string) []string {
	now := r.env.now()
	return slices.DeleteFunc(r.candidateNames(kind, exclude), func(name string) bool {
		issue, _ := r.agentAvailability(name, now)
		return issue != nil
	})
}

func (r *Router) Pick(kind api.Kind, exclude ...string) (string, *api.Error) {
	now := r.env.now()
	kindName := string(driver.ErrNotInstalled)
	var earliest time.Time
	var states []string
	for _, name := range r.candidateNames(kind, exclude) {
		issue, until := r.agentAvailability(name, now)
		if issue == nil {
			return name, nil
		}
		states = append(states, issue.Message)
		if !until.IsZero() && (earliest.IsZero() || until.Before(earliest)) {
			earliest, kindName = until, issue.Kind
		}
	}
	message := fmt.Sprintf("请运行 xcli agents 检查安装和冷却状态，并为 %s 启用可用 CLI", kind)
	if len(states) > 0 {
		message = strings.Join(states, "；")
	}
	return "", &api.Error{Kind: kindName, Message: message}
}

func (r *Router) CheckPlanAvailability(plan api.Plan) *api.Error {
	now := r.env.now()
	for _, step := range plan.Steps {
		executor := step.Agent
		if r.env.Req.Agent != "" {
			executor = r.env.Req.Agent
		}
		names := []string{executor}
		if step.ReviewBy != nil && *step.ReviewBy != executor {
			names = append(names, *step.ReviewBy)
		}
		for _, name := range names {
			if name == "auto" {
				continue
			}
			if issue, _ := r.agentAvailability(name, now); issue != nil {
				issue.StepID = step.ID
				return issue
			}
		}
	}
	return nil
}

func (r *Router) agentAvailability(name string, now time.Time) (*api.Error, time.Time) {
	if r.env.Health != nil {
		entry := r.env.Health.Entry(name)
		if entry.CooldownUntil.After(now) {
			kind := driver.ErrorKind(entry.CooldownReason)
			reason := map[driver.ErrorKind]string{
				driver.ErrAuth: "未登录", driver.ErrQuota: "额度用尽", driver.ErrUnavailable: "服务暂不可用",
			}[kind]
			if reason == "" {
				kind, reason = driver.ErrUnavailable, "服务暂不可用"
			}
			minutes := (entry.CooldownUntil.Sub(now)-1)/time.Minute + 1
			message := fmt.Sprintf("%s 正在冷却（%s），约 %d 分钟后（%s）可重试；如已处理，可运行 xcli agents --reset 立即解除",
				name, reason, minutes, entry.CooldownUntil.Local().Format("15:04"))
			return &api.Error{Kind: string(kind), Message: message}, entry.CooldownUntil
		}
	}
	command := r.env.Config.Agents[name].Command
	if d := r.env.Drivers[name]; d != nil {
		command = d.Command()
		if _, err := proc.LookPath(command, r.env.PathList); err == nil {
			return nil, time.Time{}
		}
	}
	message := fmt.Sprintf("未找到 %s（%s），请安装或在配置中修正 command", name, command)
	return &api.Error{Kind: string(driver.ErrNotInstalled), Message: message}, time.Time{}
}

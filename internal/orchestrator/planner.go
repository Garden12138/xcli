package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/driver"
	"github.com/Garden12138/xcli/internal/recipe"
	"github.com/Garden12138/xcli/internal/stats"
	"github.com/Garden12138/xcli/internal/workspace"
)

func (e *Env) Plan(ctx context.Context) api.Plan {
	e.state()
	fallback := func(reason string) api.Plan {
		e.emit("notice", "plan", "", map[string]string{"text": reason + "；已采用规则计划"})
		return RulesPlan(e.Req, e.Config)
	}
	router := NewRouter(e)
	candidates := router.Candidates(api.Kind("plan"))
	if preferred := e.Config.Defaults.Planner; e.Req.Agent == "" && slices.Contains(candidates, preferred) {
		candidates = append([]string{preferred}, slices.DeleteFunc(candidates, func(name string) bool { return name == preferred })...)
	}
	if len(candidates) == 0 {
		return fallback("请运行 xcli agents 检查规划 CLI 的安装和冷却状态")
	}
	input := recipe.PlannerInput{Goal: e.Req.Goal, Workdir: e.WS.Dir, IsGit: e.WS.Git, Listing: workspace.Listing(e.WS.Dir, 50), Inputs: append(append([]string{}, e.WS.Inputs...), e.WS.Images...), MaxSteps: e.Config.Defaults.MaxSteps}
	notes := map[string]string{
		"codex":       "OpenAI Codex：擅长编码、规划，内置 image_gen 可生图",
		"cursor":      "Cursor Agent：擅长编码与代码审查",
		"antigravity": "Google Antigravity：擅长长文本、资料分析，内置 image-generator 子代理可生图",
	}
	for attempt, name := range candidates[:min(2, len(candidates))] {
		if ctx.Err() != nil {
			return fallback("规划已取消或超时，请重新提交任务继续")
		}
		available := router.eligible()
		input.Agents = nil
		for _, name := range available {
			input.Agents = append(input.Agents, recipe.AgentInfo{Name: name, Strengths: e.Config.Agents[name].Strengths, Note: notes[name]})
		}
		prompt, err := recipe.RenderPlanner(input)
		if err != nil {
			return fallback("请检查规划提示词模板：" + err.Error())
		}
		if e.Req.Permission == api.PermRead {
			prompt += "\n本次请求权限为 read，只安排 ask 或 review 步骤。\n"
		}
		schema := recipe.PlanSchema(available, e.Config.Defaults.MaxSteps)
		runner := *e
		runner.kind = api.KindAsk
		if attempt > 0 {
			runner.logSubdir = filepath.Join(e.logSubdir, fmt.Sprintf("fallback-%d-%s", attempt, name))
		}
		dir, err := runner.logDir("plan", 1)
		if err != nil {
			return fallback("请检查规划日志目录：" + err.Error())
		}
		inv := driver.Invocation{Prompt: prompt, Workdir: e.WS.Dir, Permission: api.PermRead}
		if len(input.Inputs) > 0 && outside(e.WS.Dir, e.WS.InputsDir) {
			inv.AddDirs = []string{e.WS.InputsDir}
		}
		if e.Drivers[name].NativeSchema() {
			inv.SchemaFile = filepath.Join(dir, "schema.json")
			if err := os.WriteFile(inv.SchemaFile, schema, 0600); err != nil {
				return fallback("请检查规划 Schema 文件写入权限：" + err.Error())
			}
		} else {
			inv.Prompt += "\n请只输出符合下面 JSON Schema 的 JSON：\n" + string(schema)
		}
		planCtx, cancel := context.WithTimeout(ctx, e.Config.Defaults.PlannerTimeout.D())
		result := runner.invoke(planCtx, name, stats.PurposePlan, "plan", 1, inv)
		cancel()
		if result.ErrorKind != driver.ErrNone {
			e.emit("notice", "plan", name, map[string]string{"text": "规划 CLI " + name + "：" + result.Error})
			continue
		}
		raw, err := recipe.ExtractJSON(result.Text)
		if err == nil {
			err = recipe.Validate(schema, raw)
		}
		var plan api.Plan
		if err == nil {
			err = json.Unmarshal(raw, &plan)
		}
		if err == nil {
			err = ValidatePlan(plan, e.Req, available, e.Config.Defaults.MaxSteps)
		}
		if err != nil {
			return fallback("计划校验失败，请检查需求或使用 --plan 提供计划：" + err.Error())
		}
		plan.Source = "planner"
		plan.Planner = name
		return plan
	}
	return fallback("规划调用已结束，请查看步骤 plan 的日志或稍后重试")
}

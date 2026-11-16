package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/spf13/cobra"
)

type requestFlags struct {
	req                          api.Request
	models                       []string
	review, noReview, noFallback bool
	maxFix                       int
	timeout                      time.Duration
	planFile                     string
	changed                      func(string) bool
	outputOptions
}

func (f *requestFlags) add(c *cobra.Command) {
	p := c.Flags()
	f.changed = p.Changed
	p.StringVarP(&f.req.Workdir, "workdir", "C", "", "工作目录")
	p.StringVarP(&f.req.Agent, "agent", "a", "", "锁定 CLI")
	p.StringSliceVar(&f.req.Agents, "agents", nil, "允许的 CLI，按偏好排序")
	p.StringArrayVarP(&f.req.Inputs, "input", "i", nil, "输入文件或目录，可重复")
	p.StringArrayVar(&f.req.Images, "image", nil, "参考图，可重复")
	p.StringVarP(&f.req.OutputDir, "out", "o", "", "产物目录")
	p.StringVar(&f.req.SchemaFile, "schema", "", "JSON Schema 文件")
	p.StringVar((*string)(&f.req.Permission), "permission", "", "read/write/full，默认按配置")
	p.BoolVar(&f.req.Isolate, "isolate", false, "在 git worktree 中执行")
	p.BoolVar(&f.review, "review", false, "启用交叉审查")
	p.BoolVar(&f.noReview, "no-review", false, "关闭交叉审查")
	p.StringVar(&f.req.Reviewer, "reviewer", "", "审查 CLI")
	p.StringArrayVar(&f.req.Verify, "verify", nil, "验证命令，可重复")
	p.IntVar(&f.maxFix, "max-fix-rounds", 0, "最大修复轮数，默认按配置")
	p.StringArrayVar(&f.models, "model", nil, "agent=model，可重复")
	p.DurationVar(&f.timeout, "timeout", 0, "任务总超时，默认按配置")
	p.BoolVar(&f.noFallback, "no-fallback", false, "关闭自动切换")
	p.StringVar(&f.planFile, "plan", "", "执行指定计划 JSON 文件")
	p.BoolVar(&f.req.PlanOnly, "plan-only", false, "只生成计划")
	f.outputOptions.add(c, true)
}

func (f *requestFlags) Request(cap api.Capability, goal string) api.Request {
	r := f.req
	r.Capability, r.Goal = cap, goal
	changed := func(name string) bool { return f.changed != nil && f.changed(name) }
	if changed("review") || changed("no-review") {
		value := f.review
		if changed("no-review") {
			value = !f.noReview
		}
		r.Review = &value
	}
	if changed("no-fallback") {
		value := !f.noFallback
		r.Fallback = &value
	}
	if changed("max-fix-rounds") {
		value := f.maxFix
		r.MaxFixRounds = &value
	}
	if f.timeout > 0 {
		r.TimeoutSec = int((f.timeout + time.Second - 1) / time.Second)
	}
	r.Models = map[string]string{}
	for _, value := range f.models {
		if name, model, ok := strings.Cut(value, "="); ok {
			r.Models[name] = model
		}
	}
	return r
}

func (f *requestFlags) read(cap api.Capability, goal string) (api.Request, error) {
	r := f.Request(cap, goal)
	if f.changed("review") && f.changed("no-review") {
		return r, fmt.Errorf("请在 --review 和 --no-review 中选择一个")
	}
	if f.timeout < 0 {
		return r, fmt.Errorf("请将 --timeout 设为正时长")
	}
	for _, v := range f.models {
		a, m, ok := strings.Cut(v, "=")
		if !ok || a == "" || m == "" {
			return r, fmt.Errorf("请按 agent=model 填写 --model，例如 codex=gpt-5.5")
		}
	}
	if f.planFile != "" {
		raw, err := os.Open(f.planFile)
		if err != nil {
			return r, fmt.Errorf("请检查 --plan 文件：%w", err)
		}
		defer raw.Close()
		var plan api.Plan
		if err := decodeJSON(raw, &plan); err != nil {
			return r, err
		}
		r.Plan = &plan
	}
	return r, nil
}

func decodeJSON(r io.Reader, value any) error {
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return fmt.Errorf("请检查 JSON 请求或计划字段：%w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("请只提供一个完整的 JSON 对象")
	}
	return nil
}

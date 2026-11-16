package orchestrator

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
)

func KeywordCapability(goal string, hasWorkdir bool) api.Capability {
	goal = strings.ToLower(goal)
	for _, key := range []string{"图", "图片", "插画", "封面", "logo", "image", "画"} {
		if strings.Contains(goal, key) {
			return api.CapImage
		}
	}
	if hasWorkdir {
		for _, key := range []string{"修复", "实现", "重构", "代码", "bug", "fix", "implement", "refactor", "测试"} {
			if strings.Contains(goal, key) {
				return api.CapCode
			}
		}
	}
	return api.CapAsk
}

func RulesPlan(req api.Request, cfg config.Config) api.Plan {
	capability := req.Capability
	if capability == "" || capability == api.CapAuto {
		capability = KeywordCapability(req.Goal, req.Workdir != "")
	}
	agent := req.Agent
	if agent == "" {
		agent = "auto"
	}
	step := api.Step{ID: "main", Kind: api.Kind(capability), Agent: agent, Instruction: req.Goal, DependsOn: []string{}}
	if step.Kind == api.KindCode {
		review := cfg.Defaults.Review != "off"
		if req.Review != nil {
			review = *req.Review
		}
		if review {
			name := req.Reviewer
			if name == "" {
				name = "auto"
			}
			step.ReviewBy = &name
		}
	}
	return api.Plan{Source: "rules", Steps: []api.Step{step}}
}

var planID = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

func ValidatePlan(p api.Plan, req api.Request, available []string, maxSteps int) error {
	if len(p.Steps) == 0 || len(p.Steps) > maxSteps {
		return fmt.Errorf("请将计划步骤数设为 1 到 %d", maxSteps)
	}
	steps := map[string]api.Step{}
	for _, s := range p.Steps {
		if !planID.MatchString(s.ID) {
			return fmt.Errorf("请修改步骤 ID %q，使用 1–32 位小写字母、数字、下划线或连字符", s.ID)
		}
		if _, ok := steps[s.ID]; ok {
			return fmt.Errorf("请为重复步骤 %s 指定唯一 ID", s.ID)
		}
		steps[s.ID] = s
		switch s.Kind {
		case api.KindAsk, api.KindCode, api.KindReview, api.KindImage:
		default:
			return fmt.Errorf("请修正步骤 %s 的 kind", s.ID)
		}
		if s.Agent != "auto" && !slices.Contains(available, s.Agent) {
			return fmt.Errorf("请将步骤 %s 的 agent 设为 auto 或可用 CLI", s.ID)
		}
		if req.Agent != "" && s.Agent != "auto" && s.Agent != req.Agent {
			return fmt.Errorf("请将步骤 %s 的 agent 设为 %s 或 auto", s.ID, req.Agent)
		}
		if req.Permission == api.PermRead && (s.Kind == api.KindCode || s.Kind == api.KindImage) {
			return fmt.Errorf("步骤 %s 需要 write 或 full 权限，请调整 permission 或改用只读步骤", s.ID)
		}
		if s.ReviewBy != nil {
			if s.Kind != api.KindCode {
				return fmt.Errorf("请仅在 code 步骤设置 review_by：%s", s.ID)
			}
			if *s.ReviewBy != "auto" && !slices.Contains(available, *s.ReviewBy) {
				return fmt.Errorf("请为步骤 %s 选择可用 review_by 或 auto", s.ID)
			}
		}
		if s.Output != nil && (filepath.IsAbs(*s.Output) || filepath.Clean(*s.Output) == ".." || strings.HasPrefix(filepath.Clean(*s.Output), ".."+string(filepath.Separator))) {
			return fmt.Errorf("请将步骤 %s 的 output 设为输出目录内的相对路径", s.ID)
		}
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("请移除步骤 %s 的循环依赖", id)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, dependency := range steps[id].DependsOn {
			if _, ok := steps[dependency]; !ok {
				return fmt.Errorf("请修正步骤 %s 的依赖 %s", id, dependency)
			}
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for _, s := range p.Steps {
		if err := visit(s.ID); err != nil {
			return err
		}
	}
	return nil
}

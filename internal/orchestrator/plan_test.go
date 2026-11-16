package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
)

func TestRulesPlan(t *testing.T) {
	for _, cap := range []api.Capability{api.CapAsk, api.CapReview, api.CapImage, api.CapCode} {
		cfg := config.Default()
		p := RulesPlan(api.Request{Capability: cap, Goal: "x"}, cfg)
		if p.Source != "rules" || len(p.Steps) != 1 || p.Steps[0].ID != "main" || p.Steps[0].Agent != "auto" || (p.Steps[0].ReviewBy != nil) != (cap == api.CapCode) {
			t.Fatalf("%s: %+v", cap, p)
		}
	}
	off := false
	p := RulesPlan(api.Request{Capability: api.CapCode, Goal: "x", Review: &off}, config.Default())
	if p.Steps[0].ReviewBy != nil {
		t.Fatal("disabled review retained")
	}
	p = RulesPlan(api.Request{Capability: api.CapCode, Goal: "x", Agent: "codex", Reviewer: "cursor"}, config.Default())
	if p.Steps[0].Agent != "codex" || p.Steps[0].ReviewBy == nil || *p.Steps[0].ReviewBy != "cursor" {
		t.Fatal(p)
	}
	cfg := config.Default()
	cfg.Defaults.Review = "off"
	if p := RulesPlan(api.Request{Capability: api.CapCode, Goal: "x"}, cfg); p.Steps[0].ReviewBy != nil {
		t.Fatal(p)
	}
}

func TestKeywordCapability(t *testing.T) {
	for _, tc := range []struct {
		goal string
		dir  bool
		want api.Capability
	}{
		{"画一张猫的封面图", false, api.CapImage}, {"修复登录 bug", true, api.CapCode}, {"修复登录 bug", false, api.CapAsk}, {"总结这份文档", true, api.CapAsk},
	} {
		if got := KeywordCapability(tc.goal, tc.dir); got != tc.want {
			t.Fatalf("%s: %s", tc.goal, got)
		}
	}
}

func TestValidatePlan(t *testing.T) {
	base := api.Plan{Steps: []api.Step{{ID: "a", Kind: api.KindAsk, Agent: "auto", Instruction: "x"}, {ID: "b", Kind: api.KindAsk, Agent: "codex", Instruction: "y", DependsOn: []string{"a"}}}}
	for _, tc := range []struct {
		name    string
		change  func(*api.Plan, *api.Request)
		invalid bool
	}{
		{"valid", func(*api.Plan, *api.Request) {}, false},
		{"duplicate", func(p *api.Plan, _ *api.Request) { p.Steps[1].ID = "a" }, true},
		{"unknown dependency", func(p *api.Plan, _ *api.Request) { p.Steps[1].DependsOn = []string{"missing"} }, true},
		{"cycle", func(p *api.Plan, _ *api.Request) { p.Steps[0].DependsOn = []string{"b"} }, true},
		{"unknown agent", func(p *api.Plan, _ *api.Request) { p.Steps[0].Agent = "other" }, true},
		{"read code", func(p *api.Plan, r *api.Request) { p.Steps[0].Kind = api.KindCode; r.Permission = api.PermRead }, true},
		{"read image", func(p *api.Plan, r *api.Request) { p.Steps[0].Kind = api.KindImage; r.Permission = api.PermRead }, true},
		{"review on ask", func(p *api.Plan, _ *api.Request) { s := "cursor"; p.Steps[0].ReviewBy = &s }, true},
		{"bad id", func(p *api.Plan, _ *api.Request) { p.Steps[0].ID = "../escape" }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(base)
			var plan api.Plan
			_ = json.Unmarshal(raw, &plan)
			req := api.Request{Permission: api.PermWrite}
			tc.change(&plan, &req)
			err := ValidatePlan(plan, req, []string{"codex", "cursor"}, 6)
			if (err != nil) != tc.invalid {
				t.Fatalf("invalid=%t, error=%v", tc.invalid, err)
			}
		})
	}
	if err := ValidatePlan(base, api.Request{}, []string{"codex"}, 1); err == nil {
		t.Fatal("step limit ignored")
	}
}

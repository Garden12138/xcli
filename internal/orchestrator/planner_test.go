package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/driver"
	"github.com/Garden12138/xcli/internal/stats"
	"github.com/Garden12138/xcli/internal/store"
	"github.com/Garden12138/xcli/internal/testutil"
)

func plannerFixture() api.Plan {
	reviewer := "auto"
	return api.Plan{Reasoning: "文案和图片并行，完成后写入 README", Steps: []api.Step{
		{ID: "copy", Kind: api.KindAsk, Agent: "antigravity", Instruction: "写一段项目介绍", DependsOn: []string{}},
		{ID: "cover", Kind: api.KindImage, Agent: "auto", Instruction: "生成封面图", DependsOn: []string{}},
		{ID: "apply", Kind: api.KindCode, Agent: "codex", Instruction: "写入 README 并引用图片", DependsOn: []string{"copy", "cover"}, ReviewBy: &reviewer},
	}}
}

func plannerJSON(plan api.Plan) string {
	raw, _ := json.Marshal(struct {
		Reasoning string     `json:"reasoning"`
		Steps     []api.Step `json:"steps"`
	}{plan.Reasoning, plan.Steps})
	return string(raw)
}

func agyOK(text string) []string {
	raw, _ := json.Marshal(map[string]any{"event": "result", "result": map[string]any{"status": "SUCCESS", "response": text, "conversation_id": "plan-session"}})
	return []string{string(raw)}
}

func plannerNotices(t *testing.T, e *Env) string {
	t.Helper()
	events, err := store.ReadEvents(e.Task.Dir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, ev := range events {
		if ev.Type == "notice" {
			out.Write(ev.Data)
		}
	}
	return out.String()
}

func TestPlannerSuccess(t *testing.T) {
	want := plannerFixture()
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK(plannerJSON(want))}}}, api.Request{Capability: api.CapAuto, Workdir: codeRepo(t)})
	p := e.Plan(context.Background())
	if p.Source != "planner" || p.Reasoning != want.Reasoning || !reflect.DeepEqual(p.Steps, want.Steps) {
		t.Fatalf("plan: %+v", p)
	}
	calls := testutil.FakeCalls(t)
	canonical, _ := filepath.EvalSymlinks(e.WS.Dir)
	if len(calls) != 1 || calls[0].Name != "codex" || !strings.Contains(strings.Join(calls[0].Args, " "), "--output-schema") || !strings.Contains(strings.Join(calls[0].Args, " "), "-s read-only") || calls[0].Cwd != canonical {
		t.Fatalf("calls: %+v", calls)
	}
}

func TestPlannerInvalidFallsBack(t *testing.T) {
	want := plannerFixture()
	want.Steps[0].DependsOn = []string{"apply"}
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK(plannerJSON(want))}}}, api.Request{Capability: api.CapAuto, Goal: "总结项目"})
	p := e.Plan(context.Background())
	if p.Source != "rules" || len(p.Steps) != 1 || p.Steps[0].Kind != api.KindAsk || !strings.Contains(plannerNotices(t, e), "计划校验失败") || len(testutil.FakeCalls(t)) != 1 {
		t.Fatalf("plan: %+v notices=%s", p, plannerNotices(t, e))
	}
}

func TestPlannerUnavailableTriesNext(t *testing.T) {
	p := plannerFixture()
	p.Steps[2].Agent = "auto"
	e := stepEnv(t, testutil.FakeScript{"codex": {{Exit: 1, Stderr: "Not logged in. Run codex login"}}, "agy": {{Stdout: agyOK(plannerJSON(p))}}}, api.Request{Capability: api.CapAuto})
	got := e.Plan(context.Background())
	calls := testutil.FakeCalls(t)
	if got.Source != "planner" || got.Planner != "antigravity" || len(calls) != 2 || calls[0].Name != "codex" || calls[1].Name != "agy" || !strings.Contains(strings.Join(calls[1].Args, " "), "--json-schema") || e.Health.Available("codex", time.Now()) {
		t.Fatalf("plan: %+v calls=%+v", got, calls)
	}
	checkCalls(t, e,
		stats.Call{Agent: "codex", Purpose: "plan", Step: "plan", Round: 1, ErrorKind: "auth"},
		stats.Call{Agent: "antigravity", Purpose: "plan", Step: "plan", Round: 1},
	)
	first, _ := os.ReadFile(e.Task.Path("steps", "plan-r1", "stderr.log"))
	if !strings.Contains(string(first), "Not logged in") {
		t.Fatal("first planner logs overwritten")
	}
	if strings.Contains(calls[1].Args[len(calls[1].Args)-1], "- codex strengths=") {
		t.Fatal("second planner still describes a CLI in auth cooldown")
	}
}

func TestPlannerTimeout(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{SleepMS: 10000}}, "agy": {{SleepMS: 10000}}, "cursor-agent": {{SleepMS: 10000}}}, api.Request{Capability: api.CapAuto})
	e.Config.Defaults.PlannerTimeout = config.Duration(300 * time.Millisecond)
	start := time.Now()
	p := e.Plan(context.Background())
	calls := testutil.FakeCalls(t)
	if p.Source != "rules" || len(calls) != 2 || time.Since(start) > 2*time.Second || !strings.Contains(plannerNotices(t, e), "超时") {
		t.Fatalf("plan=%+v elapsed=%s calls=%+v notices=%s", p, time.Since(start), calls, plannerNotices(t, e))
	}
}

func TestPlannerPromptContents(t *testing.T) {
	repo := codeRepo(t)
	input := filepath.Join(t.TempDir(), "requirements.txt")
	if err := os.WriteFile(input, []byte("需求"), 0600); err != nil {
		t.Fatal(err)
	}
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK(plannerJSON(plannerFixture()))}}}, api.Request{Goal: "写项目介绍和封面图", Capability: api.CapAuto, Workdir: repo, Inputs: []string{input}})
	e.Config.Defaults.MaxSteps = 5
	e.Plan(context.Background())
	prompt, err := os.ReadFile(e.Task.Path("steps", "plan-r1", "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{e.Req.Goal, repo, "README.md", e.WS.Inputs[0], "codex", "cursor", "antigravity", "strengths=", "OpenAI Codex", "Cursor Agent", "Google Antigravity", "最多 5 步", "文案和图片互不依赖", "output 是产物子目录名", "ask 步骤只返回文本", "由下游 code 步骤写入文件"} {
		if !strings.Contains(string(prompt), part) {
			t.Fatalf("missing %q: %s", part, prompt)
		}
	}
}

func TestPlannerHonorsPreferenceAndLock(t *testing.T) {
	for _, tc := range []struct{ name, preference, locked, want string }{
		{"configured", "antigravity", "", "agy"}, {"locked", "antigravity", "codex", "codex"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := api.Plan{Reasoning: "单步骤", Steps: []api.Step{{ID: "main", Kind: api.KindAsk, Agent: "auto", Instruction: "总结", DependsOn: []string{}}}}
			e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK(plannerJSON(p))}}, "agy": {{Stdout: agyOK(plannerJSON(p))}}}, api.Request{Capability: api.CapAuto, Agent: tc.locked})
			e.Config.Defaults.Planner = tc.preference
			got := e.Plan(context.Background())
			calls := testutil.FakeCalls(t)
			if got.Source != "planner" || len(calls) != 1 || calls[0].Name != tc.want {
				t.Fatalf("plan=%+v calls=%+v", got, calls)
			}
		})
	}
}

func TestPlannerLockedFailureDoesNotCallOtherCLI(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Exit: 1, Stderr: "Not logged in"}}}, api.Request{Capability: api.CapAuto, Agent: "codex"})
	if p := e.Plan(context.Background()); p.Source != "rules" || len(testutil.FakeCalls(t)) != 1 {
		t.Fatalf("plan: %+v", p)
	}
}

func TestPlannerSchemaAndAllowedAgents(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		permission api.Permission
	}{
		{"unknown field", `{"reasoning":"x","steps":[],"unexpected":true}`, api.PermWrite},
		{"non JSON", "plain text", api.PermWrite},
		{"read cannot code", plannerJSON(api.Plan{Steps: []api.Step{{ID: "main", Kind: api.KindCode, Agent: "auto", Instruction: "change", DependsOn: []string{}}}}), api.PermRead},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK(tc.text)}}}, api.Request{Capability: api.CapAuto, Goal: "总结", Agents: []string{"codex"}, Permission: tc.permission})
			p := e.Plan(context.Background())
			if p.Source != "rules" || len(testutil.FakeCalls(t)) != 1 {
				t.Fatalf("plan: %+v", p)
			}
			raw, err := os.ReadFile(e.Task.Path("steps", "plan-r1", "schema.json"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), `"cursor"`) || strings.Contains(string(raw), `"antigravity"`) {
				t.Fatalf("schema ignored allowed agents: %s", raw)
			}
		})
	}
}

func TestPlannerNoAvailableAndCancelled(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{}, api.Request{Capability: api.CapAuto})
	e.Drivers = map[string]driver.Driver{}
	if p := e.Plan(context.Background()); p.Source != "rules" || plannerNotices(t, e) == "" {
		t.Fatalf("plan: %+v", p)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if p := e.Plan(ctx); p.Source != "rules" || len(testutil.FakeCalls(t)) != 0 {
		t.Fatalf("cancelled plan: %+v", p)
	}
}

func TestPlannerDescribesAgentsWithZeroPlanningStrength(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK(plannerJSON(plannerFixture()))}}}, api.Request{Capability: api.CapAuto})
	a := e.Config.Agents["antigravity"]
	a.Strengths["plan"] = 0
	e.Config.Agents["antigravity"] = a
	p := e.Plan(context.Background())
	prompt, _ := os.ReadFile(e.Task.Path("steps", "plan-r1", "prompt.md"))
	if p.Source != "planner" || !strings.Contains(string(prompt), "antigravity strengths=") {
		t.Fatalf("plan: %+v prompt=%s", p, prompt)
	}
}

package recipe

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Garden12138/xcli/internal/api"
)

func TestRenderAskWithInlineSchema(t *testing.T) {
	text, err := Render(Input{Kind: api.KindAsk, Agent: "antigravity", Goal: "解释", Inputs: []string{"/input/a.txt"}, InlineSchema: []byte(`{"type":"object"}`)})
	if err != nil || !strings.HasPrefix(text, "# xcli 任务") {
		t.Fatalf("render: %s, %v", text, err)
	}
	for _, want := range []string{"/input/a.txt", `{"type":"object"}`, "view_file", "只读", "git commit", "与需求相同的语言"} {
		if !strings.Contains(text, want) {
			t.Errorf("prompt missing %s", want)
		}
	}
}

func TestRenderFixUsedForRound2(t *testing.T) {
	text, err := Render(Input{Kind: api.KindCode, Round: 2, Feedback: "go test failed"})
	if err != nil || !strings.Contains(text, "第 2 轮") || !strings.Contains(text, "go test failed") {
		t.Fatalf("fix: %s, %v", text, err)
	}
}

func TestRenderImageHasOutputLine(t *testing.T) {
	text, err := Render(Input{Kind: api.KindImage, Agent: "codex", OutputDir: "/out", Images: []string{"/in.png"}})
	if err != nil || !strings.Contains(text, "\n输出目录：/out\n") || !strings.Contains(text, "image_gen") || !strings.Contains(text, "/in.png") {
		t.Fatalf("image: %s, %v", text, err)
	}
}

func TestRenderHandoffIsBoundedAndPure(t *testing.T) {
	up := []Handoff{{StepID: "a", Agent: "codex", Kind: api.KindAsk, Summary: strings.Repeat("界", 3000), Artifacts: []string{"/out/a.png"}, Files: []string{"a.go"}}}
	text, err := Render(Input{Kind: api.KindCode, Upstream: up})
	if err != nil || !utf8.ValidString(text) || strings.Contains(text, up[0].Summary) || !strings.Contains(text, "/out/a.png") || !strings.Contains(text, "a.go") || !strings.Contains(text, "上游步骤 a") {
		t.Fatalf("handoff: bytes=%d, %v", len(text), err)
	}
	if len(up[0].Summary) != 9000 {
		t.Fatal("render mutated upstream data")
	}
}

func TestRenderPlanner(t *testing.T) {
	text, err := RenderPlanner(PlannerInput{Goal: "任务", Workdir: "/repo", IsGit: true, Listing: []string{"README.md"}, Inputs: []string{"/input"}, Agents: []AgentInfo{{Name: "codex", Strengths: map[string]int{"plan": 3}, Note: "擅长规划"}}, MaxSteps: 4})
	for _, want := range []string{"任务", "/repo", "README.md", "/input", "codex", "plan:3", "4", "内置", "JSON"} {
		if err != nil || !strings.Contains(text, want) {
			t.Errorf("planner missing %s, err=%v", want, err)
		}
	}
}

func TestExtractJSON(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{`{"a":1}`, `{"a":1}`}, {"```json\n{\"a\":1}\n```", `{"a":1}`},
		{"结果如下：{\"a\":1} 完成。", `{"a":1}`}, {`[1,2]`, `[1,2]`},
		{`[not json] 然后 {"a":"]}"}`, `{"a":"]}"}`}, {"9007199254740993", "9007199254740993"},
	} {
		raw, err := ExtractJSON(tc.text)
		if err != nil || string(raw) != tc.want {
			t.Errorf("extract %q = %s, %v", tc.text, raw, err)
		}
	}
	if _, err := ExtractJSON("这里没有 JSON"); err == nil {
		t.Fatal("missing JSON passed")
	}
}

func TestValidate(t *testing.T) {
	if err := Validate(ReviewSchema(), json.RawMessage(`{"verdict":"pass","summary":"","issues":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := Validate(ReviewSchema(), json.RawMessage(`{"verdict":"maybe"}`)); err == nil {
		t.Fatal("invalid review passed")
	}
	if err := CheckSchema([]byte(`{"type":"bogus"}`)); err == nil {
		t.Fatal("invalid schema passed")
	}
	if err := Validate([]byte(`{"const":9007199254740993}`), json.RawMessage(`9007199254740993`)); err != nil {
		t.Fatalf("integer precision lost: %v", err)
	}
}

func TestBuiltinSchemasStrict(t *testing.T) {
	var check func(any)
	check = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if v["type"] == "object" {
				if v["additionalProperties"] != false {
					t.Error("object allows extra properties")
				}
				props := v["properties"].(map[string]any)
				required := map[string]bool{}
				for _, key := range v["required"].([]any) {
					required[key.(string)] = true
				}
				for key := range props {
					if !required[key] {
						t.Errorf("missing required %s", key)
					}
				}
			}
			for _, item := range v {
				check(item)
			}
		case []any:
			for _, item := range v {
				check(item)
			}
		}
	}
	for _, raw := range [][]byte{ReviewSchema(), ImageResultSchema(), PlanSchema([]string{"codex"}, 6)} {
		if err := CheckSchema(raw); err != nil {
			t.Fatal(err)
		}
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		check(doc)
	}
}

func TestPlanSchemaAgentsEnum(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(PlanSchema([]string{"codex", "cursor"}, 4), &doc); err != nil {
		t.Fatal(err)
	}
	steps := doc["properties"].(map[string]any)["steps"].(map[string]any)
	agents := steps["items"].(map[string]any)["properties"].(map[string]any)["agent"].(map[string]any)["enum"]
	if !reflect.DeepEqual(agents, []any{"auto", "codex", "cursor"}) || steps["maxItems"] != float64(4) {
		t.Fatalf("plan schema: %s", PlanSchema([]string{"codex", "cursor"}, 4))
	}
}

package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/store"
	"github.com/Garden12138/xcli/internal/testutil"
)

func TestExecuteParallel(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK("ok"), SleepMS: 500}}}, api.Request{})
	p := api.Plan{Steps: []api.Step{{ID: "a", Kind: api.KindAsk, Agent: "codex"}, {ID: "b", Kind: api.KindAsk, Agent: "codex"}}}
	start := time.Now()
	r := Execute(context.Background(), e, p)
	started0, err0 := os.Stat(os.Getenv("XCLI_FAKE_STATE") + "/codex-0.pid")
	started1, err1 := os.Stat(os.Getenv("XCLI_FAKE_STATE") + "/codex-1.pid")
	if err0 != nil || err1 != nil {
		t.Fatalf("fake process start: %v %v", err0, err1)
	}
	delta := started1.ModTime().Sub(started0.ModTime())
	if delta < 0 {
		delta = -delta
	}
	if r.Status != api.StatusCompleted || len(r.Steps) != 2 || delta >= 400*time.Millisecond {
		t.Fatalf("parallel: elapsed=%s result=%+v", time.Since(start), r)
	}
}

func TestExecuteDependencyHandoff(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK("upstream result")}, {Stdout: codexOK("terminal result")}}}, api.Request{})
	p := api.Plan{Steps: []api.Step{{ID: "a", Kind: api.KindAsk, Agent: "codex"}, {ID: "b", Kind: api.KindAsk, Agent: "codex", DependsOn: []string{"a"}}}}
	r := Execute(context.Background(), e, p)
	prompt, _ := os.ReadFile(e.Task.Path("steps", "b-r1", "prompt.md"))
	if r.Status != api.StatusCompleted || r.Summary != "terminal result" || !strings.Contains(string(prompt), "upstream result") {
		t.Fatalf("handoff: %+v, prompt=%s", r, prompt)
	}
}

func TestExecuteRetainsImageWhenReadStepDenied(t *testing.T) {
	fallback := false
	e := stepEnv(t, testutil.FakeScript{
		"agy":   {{Stdout: []string{`{"event":"result","result":{"status":"SUCCESS","response":"","denied_actions":[{"action":"read_file","display_name":"ReadFile"}]}}`}}},
		"codex": {{Stdout: codexOK(`{"files":[]}`), WritePNG: "{{OUT}}/cover.png"}},
	}, api.Request{Fallback: &fallback})
	p := api.Plan{Steps: []api.Step{
		{ID: "intro", Kind: api.KindAsk, Agent: "antigravity"},
		{ID: "cover", Kind: api.KindImage, Agent: "codex"},
		{ID: "apply", Kind: api.KindCode, Agent: "cursor", DependsOn: []string{"intro", "cover"}},
	}}
	r := Execute(context.Background(), e, p)
	if r.Status != api.StatusFailed || r.Error == nil || r.Error.Kind != "permission_denied" || len(r.Artifacts) != 1 || r.Artifacts[0].MIME != "image/png" || r.Steps[1].Status != api.StatusCompleted || r.Steps[2].Status != api.StatusSkipped || len(testutil.FakeCalls(t)) != 2 {
		t.Fatalf("read denial: %+v", r)
	}
}

func TestExecuteFallbackOnReadPermissionDenied(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{
		"agy":   {{Stdout: []string{`{"event":"result","result":{"status":"SUCCESS","response":"","denied_actions":[{"action":"read_file","display_name":"ReadFile"}]}}`}}},
		"codex": {{Stdout: codexOK("read fallback success")}},
	}, api.Request{Agents: []string{"antigravity", "codex"}})
	r := Execute(context.Background(), e, api.Plan{Steps: []api.Step{{ID: "main", Kind: api.KindAsk, Agent: "antigravity"}}})
	calls := testutil.FakeCalls(t)
	if r.Status != api.StatusCompleted || len(r.Steps) != 1 || r.Steps[0].Agent != "codex" || !reflect.DeepEqual(r.Steps[0].Fallbacks, []string{"antigravity: permission_denied"}) || len(calls) != 2 || calls[0].Name != "agy" || calls[1].Name != "codex" {
		t.Fatalf("read permission fallback: %+v, calls=%+v", r, calls)
	}
	if !strings.Contains(strings.Join(calls[1].Args, " "), "-s read-only") {
		t.Fatal("fallback must retain read-only permission")
	}
}

func TestExecutePropagatesFailureInReversePlanOrder(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Exit: 1}}}, api.Request{})
	p := api.Plan{Steps: []api.Step{
		{ID: "c", Kind: api.KindAsk, Agent: "codex", DependsOn: []string{"b"}},
		{ID: "b", Kind: api.KindAsk, Agent: "codex", DependsOn: []string{"a"}},
		{ID: "a", Kind: api.KindAsk, Agent: "codex"},
	}}
	r := Execute(context.Background(), e, p)
	if r.Status != api.StatusFailed || len(r.Steps) != 3 || r.Steps[0].Status != api.StatusSkipped || r.Steps[1].Status != api.StatusSkipped || r.Error == nil || r.Error.Kind == "invalid_request" {
		t.Fatalf("failure propagation: %+v", r)
	}
}

func TestExecutePersistsUnavailableStep(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{}, api.Request{})
	e.Drivers = nil
	r := Execute(context.Background(), e, api.Plan{Steps: []api.Step{{ID: "main", Kind: api.KindAsk, Agent: "auto"}}})
	record := e.Task.Snapshot()
	if r.Status != api.StatusFailed || len(record.Steps) != 1 || record.Steps[0].Status != api.StatusFailed || record.Steps[0].Error == nil {
		t.Fatalf("unavailable: result=%+v record=%+v", r, record.Steps)
	}
}

func TestExecuteSkipsDependents(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Exit: 1}}, "cursor-agent": {{Stdout: cursorOK("unrelated")}}}, api.Request{})
	p := api.Plan{Steps: []api.Step{{ID: "a", Kind: api.KindAsk, Agent: "codex"}, {ID: "b", Kind: api.KindAsk, Agent: "codex", DependsOn: []string{"a"}}, {ID: "c", Kind: api.KindAsk, Agent: "cursor"}}}
	r := Execute(context.Background(), e, p)
	if r.Status != api.StatusFailed || r.Steps[1].Status != api.StatusSkipped || r.Steps[2].Status != api.StatusCompleted || len(testutil.FakeCalls(t)) != 2 {
		t.Fatalf("dependency failure: %+v", r)
	}
}

func TestExecuteSerializesCodeOnSameWorkdir(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK("done"), SleepMS: 300}}}, api.Request{Workdir: t.TempDir()})
	p := api.Plan{Steps: []api.Step{{ID: "a", Kind: api.KindCode, Agent: "codex"}, {ID: "b", Kind: api.KindCode, Agent: "codex"}}}
	start := time.Now()
	r := Execute(context.Background(), e, p)
	if r.Status != api.StatusCompleted || time.Since(start) < 600*time.Millisecond {
		t.Fatalf("code serialization: elapsed=%s result=%+v", time.Since(start), r)
	}
}

func TestExecuteFallbackOnQuota(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: []string{`{"type":"turn.failed","error":{"message":"quota exceeded"}}`}, Exit: 1}}, "cursor-agent": {{Stdout: cursorOK("fallback success")}}}, api.Request{Agents: []string{"codex", "cursor"}})
	r := Execute(context.Background(), e, api.Plan{Steps: []api.Step{{ID: "main", Kind: api.KindAsk, Agent: "auto"}}})
	if r.Status != api.StatusCompleted || r.Steps[0].Agent != "cursor" || !reflect.DeepEqual(r.Steps[0].Fallbacks, []string{"codex: quota"}) || e.Health.Available("codex", time.Now()) {
		t.Fatalf("fallback: %+v", r)
	}
	if got := e.Calls(); len(got) != 2 || got[0].Agent != "codex" || got[0].Purpose != "run" || got[0].ErrorKind != "quota" ||
		got[1].Agent != "cursor" || got[1].Purpose != "run" || got[1].ErrorKind != "" {
		t.Fatalf("fallback calls: %+v", got)
	}
	events, err := store.ReadEvents(e.Task.Dir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.Type == "agent.fallback" {
			found = true
		}
	}
	if !found {
		t.Fatal("fallback event missing")
	}
}

func TestExecuteFallbackOnUnavailable(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("..", "driver", "testdata", "antigravity", "error_capacity_with_image.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	e := stepEnv(t, testutil.FakeScript{
		"agy":   {{StdoutFile: fixture, WritePNG: "{{OUT}}/failed.png", Exit: 1}},
		"codex": {{Stdout: codexOK(`{"files":[]}`), WritePNG: "{{OUT}}/from-codex.png"}},
	}, api.Request{Capability: api.CapImage})
	r := Execute(context.Background(), e, api.Plan{Steps: []api.Step{{ID: "cover", Kind: api.KindImage, Agent: "antigravity"}}})
	calls := testutil.FakeCalls(t)
	if r.Status != api.StatusCompleted || len(r.Steps) != 1 || r.Steps[0].Agent != "codex" || !reflect.DeepEqual(r.Steps[0].Fallbacks, []string{"antigravity: unavailable"}) || len(calls) != 2 || calls[0].Name != "agy" || calls[1].Name != "codex" || e.Health.Available("antigravity", time.Now()) {
		t.Fatalf("service unavailable fallback: %+v, calls=%+v", r, calls)
	}
	if len(r.Artifacts) != 1 || r.Artifacts[0].Agent != "codex" {
		t.Fatalf("fallback artifact: %+v", r.Artifacts)
	}
}

func TestExecuteNoFallbackWhenLocked(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: []string{`{"type":"turn.failed","error":{"message":"quota exceeded"}}`}, Exit: 1}}}, api.Request{Agent: "codex"})
	r := Execute(context.Background(), e, api.Plan{Steps: []api.Step{{ID: "main", Kind: api.KindAsk, Agent: "codex"}}})
	if r.Status != api.StatusFailed || len(testutil.FakeCalls(t)) != 1 || len(r.Steps[0].Fallbacks) != 0 {
		t.Fatalf("locked: %+v", r)
	}
}

func TestExecuteSummaryAndStatus(t *testing.T) {
	zero := 0
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK("needs fix")}}, "cursor-agent": {{Stdout: cursorOK("independent summary")}}}, api.Request{Workdir: t.TempDir(), Verify: []string{"false"}, MaxFixRounds: &zero})
	p := api.Plan{Steps: []api.Step{{ID: "a", Kind: api.KindCode, Agent: "codex"}, {ID: "b", Kind: api.KindAsk, Agent: "cursor"}}}
	r := Execute(context.Background(), e, p)
	if r.Status != api.StatusNeedsAttention || !strings.Contains(r.Summary, "needs fix") || !strings.Contains(r.Summary, "independent summary") {
		t.Fatalf("summary: %+v", r)
	}
}

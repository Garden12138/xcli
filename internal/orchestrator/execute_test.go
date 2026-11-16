package orchestrator

import (
	"context"
	"os"
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
	if r.Status != api.StatusCompleted || len(r.Steps) != 2 || time.Since(start) >= 900*time.Millisecond {
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

package stats

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/store"
)

func TestNormalizeApp(t *testing.T) {
	for input, want := range map[string]string{
		"  WorkLog \t": "worklog", "CHAT-INSIGHT": "chat-insight",
		"a.b_c-0": "a.b_c-0", "Bad!": "unknown", "": "unknown", ".bad": "unknown",
		"abcdefghijklmnopqrstuvwxyz012345678901234": "unknown",
	} {
		if got := NormalizeApp(input); got != want {
			t.Errorf("NormalizeApp(%q) = %q, want %q", input, got, want)
		}
	}
}

func fixtureTask() (store.Record, []api.Event) {
	plan := &api.Plan{Planner: "antigravity", Reasoning: "private reasoning"}
	req := api.Request{Capability: api.CapAuto, Goal: "private goal", Workdir: "/private/work",
		ParentTask: "parent", Metadata: map[string]string{"app": " WorkLog ", "other": "private metadata"}}
	result := &api.Result{Capability: api.CapAuto, Status: api.StatusFailed, Plan: plan, Summary: "private summary",
		Steps: []api.StepResult{
			{ID: "code", Kind: api.KindCode, Agent: "codex", Status: api.StatusNeedsAttention, Rounds: 2, DurationMS: 500,
				Fallbacks: []string{"antigravity: quota"}, Usage: &api.Usage{InputTokens: 12, OutputTokens: 2}},
			{ID: "review", Kind: api.KindReview, Agent: "cursor", Status: api.StatusCompleted, Rounds: 1, DurationMS: 30, Usage: &api.Usage{}},
			{ID: "ask", Kind: api.KindAsk, Agent: "codex", Status: api.StatusFailed, Rounds: 1, DurationMS: 40, Error: &api.Error{Kind: "timeout"}},
			{ID: "skipped", Kind: api.KindAsk, Agent: "codex", Status: api.StatusSkipped},
		},
		Error: &api.Error{Kind: "timeout", StepID: "ask", Message: "private details"}}
	events := []api.Event{
		{Type: "review.result", Step: "code", Agent: "cursor", Data: json.RawMessage(`{"round":1,"verdict":"changes_requested","issues":["private issue"]}`)},
		{Type: "review.result", Step: "code", Agent: "cursor", Data: json.RawMessage(`{"round":2,"verdict":"pass"}`)},
		{Type: "review.result", Step: "review", Agent: "cursor", Data: json.RawMessage(`{"round":1,"verdict":"pass"}`)},
		{Type: "step.progress", Step: "review", Agent: "cursor", Data: json.RawMessage(`{"text":"private output"}`)},
	}
	return store.Record{ID: "task", Status: result.Status, Request: req, Plan: plan, Result: result}, events
}

func TestFromTaskExact(t *testing.T) {
	task, events := fixtureTask()
	calls := []Call{{Agent: "cursor", Purpose: "repair", Step: "code", Round: 3, DurationMS: 99, ErrorKind: "cancelled", HasUsage: true}}
	r, ok := FromTask(task, events, calls)
	if !ok || r.Approximate || !reflect.DeepEqual(r.Calls, calls) || r.App != "worklog" ||
		r.Planner != "antigravity" || r.ParentTask != "parent" || r.ErrorKind != "timeout" || r.ErrorStep != "ask" {
		t.Fatalf("record = %+v, ok = %t", r, ok)
	}
	if len(r.Steps) != 4 || !reflect.DeepEqual(r.Steps[0].Fallbacks, []Fallback{{Agent: "antigravity", Reason: "quota"}}) ||
		!r.Steps[0].HasUsage || r.Steps[0].InputTokens != 12 || !r.Steps[1].HasUsage || r.Steps[2].HasUsage ||
		!reflect.DeepEqual(r.Reviews, []ReviewRecord{{Step: "code", Agent: "cursor", Round: 1, Verdict: "changes_requested"}, {Step: "code", Agent: "cursor", Round: 2, Verdict: "pass"}, {Step: "review", Agent: "cursor", Round: 1, Verdict: "pass"}}) {
		t.Fatalf("steps = %+v, reviews = %+v", r.Steps, r.Reviews)
	}
	task.Request.Metadata = nil
	r, ok = FromTask(task, nil, []Call{})
	if !ok || r.App != "unknown" || r.Approximate || r.Calls == nil || len(r.Calls) != 0 {
		t.Fatalf("empty exact record = %+v", r)
	}
	for _, status := range []api.Status{api.StatusRunning, api.StatusQueued, api.StatusInvalid} {
		task.Status = status
		if _, ok := FromTask(task, nil, nil); ok {
			t.Errorf("recorded %s task", status)
		}
	}
}

func TestFromTaskEstimated(t *testing.T) {
	task, events := fixtureTask()
	r, ok := FromTask(task, events, nil)
	if !ok || !r.Approximate {
		t.Fatalf("record = %+v, ok = %t", r, ok)
	}
	want := []Call{
		{Agent: "antigravity", Purpose: "plan", Step: "plan", Round: 1},
		{Agent: "antigravity", Purpose: "run", Step: "code", Round: 1, ErrorKind: "quota"},
		{Agent: "codex", Purpose: "repair", Step: "code", Round: 1},
		{Agent: "codex", Purpose: "repair", Step: "code", Round: 2, InputTokens: 12, OutputTokens: 2, HasUsage: true},
		{Agent: "cursor", Purpose: "review", Step: "review", Round: 1, DurationMS: 30, HasUsage: true},
		{Agent: "codex", Purpose: "run", Step: "ask", Round: 1, DurationMS: 40, ErrorKind: "timeout"},
		{Agent: "cursor", Purpose: "review", Step: "code", Round: 1},
		{Agent: "cursor", Purpose: "review", Step: "code", Round: 2},
	}
	if !reflect.DeepEqual(r.Calls, want) {
		t.Fatalf("calls = %+v, want %+v", r.Calls, want)
	}
}

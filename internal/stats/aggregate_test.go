package stats

import (
	"slices"
	"testing"
	"time"
)

func TestAggregateMetrics(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	day := func(n int) time.Time { return time.Date(2026, 11, n, 0, 0, 0, 0, loc) }
	exact := Record{
		App: "worklog", Status: "completed", FinishedAt: day(19).Add(time.Hour).UTC(), DurationMS: 30,
		Steps: []StepRecord{{ID: "main", Kind: "code", Agent: "codex", Status: "completed", DurationMS: 999,
			Fallbacks: []Fallback{{Agent: "cursor", Reason: "quota"}}, HasUsage: true, InputTokens: 5, OutputTokens: 7}},
		Calls: []Call{
			{Agent: "cursor", Purpose: "run", Step: "main", DurationMS: 40, ErrorKind: "quota"},
			{Agent: "codex", Purpose: "run", Step: "main", DurationMS: 20, HasUsage: true, InputTokens: 100, OutputTokens: 30},
			{Agent: "cursor", Purpose: "review", Step: "main", DurationMS: 30},
			{Agent: "cursor", Purpose: "review", Step: "main", DurationMS: 3, ErrorKind: "cancelled"},
			{Agent: "antigravity", Purpose: "plan"},
		},
	}
	task, events := fixtureTask()
	old, _ := FromTask(task, events, nil)
	old.App, old.ErrorKind, old.FinishedAt, old.DurationMS = "worklog", "timeout", day(21).UTC(), 10
	report := Aggregate([]Record{
		exact, old,
		{App: "worklog", Status: "planned", StartedAt: day(21), DurationMS: 20},
		{App: "worklog", Status: "needs_attention", FinishedAt: day(21), DurationMS: 40},
		{App: "worklog", Status: "cancelled", FinishedAt: day(21), DurationMS: 999},
		{App: "gardenflow", Status: "cancelled", FinishedAt: day(21), DurationMS: 999},
		{App: "worklog", Status: "completed", FinishedAt: day(22), DurationMS: 999},
	}, Filter{Since: day(19), Until: day(22).Add(-time.Nanosecond), App: "worklog", Location: loc})
	o := report.Overview
	if o.Tasks != 5 || o.Succeeded != 2 || o.NeedsAttention != 1 || o.Failed != 1 || o.Cancelled != 1 ||
		o.SuccessRate == nil || *o.SuccessRate != 0.5 || o.InputTokens != 17 || o.OutputTokens != 9 || o.StepsWithUsage != 3 || o.Steps != 5 {
		t.Fatalf("overview: %+v", o)
	}
	if len(report.Apps) != 1 {
		t.Fatalf("app filter: %+v", report.Apps)
	}
	app := report.Apps[0]
	if app.App != "worklog" || app.AvgMS != 25 || app.P50MS != 20 || app.P90MS != 40 || !app.LastUsed.Equal(day(21)) {
		t.Fatalf("app metrics: %+v", app)
	}
	agents := map[string]AgentReport{}
	for _, agent := range report.Agents {
		agents[agent.Agent] = agent
	}
	if a := agents["codex"]; a.Calls != 4 || a.Succeeded != 3 || a.Failed != 1 || *a.SuccessRate != 0.75 ||
		a.AvgMS != 30 || a.P90MS != 40 || a.FallbackTo != 1 || a.InputTokens != 112 || a.OutputTokens != 32 {
		t.Fatalf("codex: %+v", a)
	}
	if a := agents["cursor"]; a.Calls != 6 || a.Succeeded != 4 || a.Failed != 2 || a.FallbackFrom != 1 || a.AvgMS != 25 || a.P90MS != 40 || !a.HasUsage {
		t.Fatalf("cursor: %+v", a)
	}
	if a := agents["antigravity"]; a.Calls != 3 || a.AvgMS != 0 || !a.HasDuration || a.FallbackFrom != 1 {
		t.Fatalf("antigravity: %+v", a)
	}
	if len(report.Errors) != 1 || report.Errors[0].Kind != "timeout" || report.Errors[0].Share != 1 ||
		!slices.Equal(report.Errors[0].Apps, []string{"worklog"}) || !slices.Equal(report.Errors[0].Agents, []string{"codex"}) {
		t.Fatalf("errors: %+v", report.Errors)
	}
	if len(report.Daily) != 3 || report.Daily[0].Date != "2026-11-19" || report.Daily[0].Tasks != 1 ||
		report.Daily[1].Date != "2026-11-20" || report.Daily[1].Tasks != 0 ||
		report.Daily[2].Date != "2026-11-21" || report.Daily[2].Tasks != 4 || report.Daily[2].AvgMS != 23 {
		t.Fatalf("daily: %+v", report.Daily)
	}
}

func TestAggregateAllAndCancelled(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	finished := time.Date(2026, 11, 19, 10, 0, 0, 0, loc)
	report := Aggregate([]Record{{App: "terminal", Status: "cancelled", FinishedAt: finished}},
		Filter{Until: finished.AddDate(0, 0, 1), Location: loc})
	if report.Overview.Tasks != 1 || report.Overview.SuccessRate != nil || report.Apps[0].SuccessRate != nil ||
		report.Apps[0].HasDuration || report.Range.Since.Hour() != 0 || len(report.Daily) != 2 {
		t.Fatalf("all with cancellation: %+v", report)
	}
}

func TestAggregateIncompleteStepHasNoEstimatedDuration(t *testing.T) {
	task, _ := fixtureTask()
	task.Result.Error.Kind = "worker_lost"
	task.Result.FinishedAt = time.Now()
	task.Result.Steps[0].Status, task.Result.Steps[0].DurationMS, task.Result.Steps[0].Rounds = "running", 0, 1
	task.Result.Steps[0].Fallbacks = nil
	task.Result.Steps = task.Result.Steps[:1]
	record, _ := FromTask(task, nil, nil)
	report := Aggregate([]Record{record}, Filter{Until: task.Result.FinishedAt})
	for _, agent := range report.Agents {
		if agent.Agent == "codex" && agent.HasDuration {
			t.Fatalf("included timing for incomplete step: %+v", agent)
		}
	}
}

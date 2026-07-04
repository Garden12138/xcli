package usage

import (
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/runstore"
)

func TestBuildAggregatesRunAndWorkflowUsage(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	cost := 0.1
	records := []runstore.Record{
		{ID: "run-codex", Kind: "run", Agent: "codex", StartedAt: now.Add(-time.Hour)},
		{ID: "run-claude", Kind: "run", Agent: "claude", StartedAt: now.Add(-2 * time.Hour), Usage: &agent.Usage{
			InputTokens: 10, OutputTokens: 2, TotalTokens: 12, EstimatedCostUSD: &cost,
		}},
		{ID: "run-generic", Kind: "run", Agent: "fake", StartedAt: now.Add(-3 * time.Hour)},
		{ID: "use-codex", Kind: "use", Agent: "codex", StartedAt: now.Add(-time.Hour), Usage: &agent.Usage{TotalTokens: 999}},
		{ID: "workflow", Kind: "workflow", StartedAt: now.Add(-4 * time.Hour), Steps: []runstore.StepRecord{
			{ID: "one", Agent: "codex", Attempts: 2, StartedAt: now.Add(-3 * time.Hour), Usage: &agent.Usage{InputTokens: 20, CacheReadTokens: 5, OutputTokens: 3, TotalTokens: 28}},
			{ID: "two", Agent: "claude", Attempts: 0, Usage: &agent.Usage{TotalTokens: 999}},
			{ID: "three", Agent: "gemini", Attempts: 1, Usage: &agent.Usage{InputTokens: 30, ReasoningTokens: 4, TotalTokens: 34}},
		}},
		{ID: "long-workflow", Kind: "workflow", StartedAt: now.Add(-8 * 24 * time.Hour), Steps: []runstore.StepRecord{
			{ID: "recent", Agent: "gemini", Attempts: 1, StartedAt: now.Add(-time.Hour)},
		}},
		{ID: "old", Kind: "run", Agent: "opencode", StartedAt: now.Add(-8 * 24 * time.Hour), Usage: &agent.Usage{TotalTokens: 500}},
	}

	report, err := Build(records, Options{Days: 7, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if report.Since == nil || !report.Since.Equal(now.Add(-7*24*time.Hour)) {
		t.Fatalf("unexpected since: %v", report.Since)
	}
	wantAgents := []string{"claude", "codex", "fake", "gemini"}
	if len(report.ByAgent) != len(wantAgents) {
		t.Fatalf("unexpected rows: %#v", report.ByAgent)
	}
	for index, name := range wantAgents {
		if report.ByAgent[index].Agent != name {
			t.Fatalf("row %d agent = %q, want %q", index, report.ByAgent[index].Agent, name)
		}
	}
	if report.Totals.Tasks != 6 || report.Totals.TrackedTasks != 3 || report.Totals.CostedTasks != 1 {
		t.Fatalf("unexpected totals: %#v", report.Totals)
	}
	if report.Totals.Usage.InputTokens != 60 || report.Totals.Usage.TotalTokens != 74 {
		t.Fatalf("unexpected usage totals: %#v", report.Totals.Usage)
	}
	if report.Totals.Usage.EstimatedCostUSD == nil || *report.Totals.Usage.EstimatedCostUSD != cost {
		t.Fatalf("unexpected cost totals: %#v", report.Totals.Usage)
	}
}

func TestBuildFiltersAgentAndIncludesAllTime(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	records := []runstore.Record{
		{Kind: "run", Agent: "codex", StartedAt: now.Add(-400 * 24 * time.Hour)},
		{Kind: "run", Agent: "claude", StartedAt: now},
		{Kind: "workflow", StartedAt: now, Steps: []runstore.StepRecord{{Agent: "codex", Attempts: 1}}},
	}
	report, err := Build(records, Options{Agent: "codex", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if report.Since != nil || report.Agent != "codex" || len(report.ByAgent) != 1 || report.Totals.Tasks != 2 {
		t.Fatalf("unexpected filtered report: %#v", report)
	}
}

func TestBuildRejectsInvalidDays(t *testing.T) {
	if _, err := Build(nil, Options{Days: -1}); err == nil {
		t.Fatal("expected negative days error")
	}
}

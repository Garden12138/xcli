package routing

import (
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/config"
)

func TestSelectUsesFirstMatchingRule(t *testing.T) {
	cfg := config.Defaults()
	cfg.DefaultAgent = "codex"
	cfg.Routing.Rules = []config.RouteRule{
		{Name: "first", PromptRegex: "review", Agent: "claude"},
		{Name: "second", PromptRegex: "review", Agent: "gemini"},
	}
	decision, err := Select(cfg, "please review this change")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Agent != "claude" || decision.Source != SourceRule || decision.Rule != "first" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestSelectFallsBackToDefault(t *testing.T) {
	cfg := config.Defaults()
	cfg.DefaultAgent = "codex"
	cfg.Routing.Rules = []config.RouteRule{{Name: "review", PromptRegex: "review", Agent: "claude"}}
	decision, err := Select(cfg, "implement a feature")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Agent != "codex" || decision.Source != SourceDefault || decision.Rule != "" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestSelectFailsWithoutMatchOrDefault(t *testing.T) {
	cfg := config.Defaults()
	cfg.Routing.Rules = []config.RouteRule{{Name: "review", PromptRegex: "review", Agent: "claude"}}
	_, err := Select(cfg, "implement a feature")
	if err == nil || !strings.Contains(err.Error(), "no routing rule matched") {
		t.Fatalf("expected no-route error, got %v", err)
	}
}

func TestSelectReportsInvalidRegex(t *testing.T) {
	cfg := config.Defaults()
	cfg.Routing.Rules = []config.RouteRule{{Name: "broken", PromptRegex: "[", Agent: "claude"}}
	_, err := Select(cfg, "anything")
	if err == nil || !strings.Contains(err.Error(), "invalid prompt_regex") {
		t.Fatalf("expected regex error, got %v", err)
	}
}

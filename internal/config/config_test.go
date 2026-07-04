package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMergesBuiltinsAndCustomAgent(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yaml")
	data := `version: 1
default_agent: custom
agents:
  custom:
    adapter: generic
    command: fake-agent
    run_args: ["run", "{{ prompt }}"]
    output: text
networks:
  direct:
    unset: [HTTP_PROXY]
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Agents["codex"]; !ok {
		t.Fatal("built-in codex agent was not retained")
	}
	if cfg.DefaultAgent != "custom" || cfg.Agents["custom"].RunArgs[1] != "{{ prompt }}" {
		t.Fatalf("unexpected merged config: %#v", cfg)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nunknown: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("expected strict-field error, got %v", err)
	}
}

func TestLoadRoutingRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := `version: 1
default_agent: codex
routing:
  rules:
    - name: review
      prompt_regex: '(?i)(review|审查)'
      agent: claude
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Routing.Rules) != 1 || cfg.Routing.Rules[0].Name != "review" || cfg.Routing.Rules[0].Agent != "claude" {
		t.Fatalf("unexpected routing config: %#v", cfg.Routing)
	}
}

func TestLoadRejectsUnknownRoutingFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := "version: 1\nrouting:\n  rules:\n    - name: review\n      prompt_regex: review\n      matcher: contains\n      agent: claude\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "field matcher not found") {
		t.Fatalf("expected strict routing-field error, got %v", err)
	}
}

func TestValidateRoutingRules(t *testing.T) {
	tests := []struct {
		name  string
		rules []RouteRule
		want  string
	}{
		{name: "empty regex", rules: []RouteRule{{Name: "review", Agent: "claude"}}, want: "empty prompt_regex"},
		{name: "invalid regex", rules: []RouteRule{{Name: "review", PromptRegex: "[", Agent: "claude"}}, want: "invalid prompt_regex"},
		{name: "duplicate name", rules: []RouteRule{
			{Name: "review", PromptRegex: "review", Agent: "claude"},
			{Name: "review", PromptRegex: "audit", Agent: "codex"},
		}, want: "duplicate routing rule name"},
		{name: "unknown agent", rules: []RouteRule{{Name: "review", PromptRegex: "review", Agent: "missing"}}, want: "references unknown agent"},
		{name: "invalid name", rules: []RouteRule{{Name: "review rule", PromptRegex: "review", Agent: "claude"}}, want: "invalid name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Routing.Rules = test.rules
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestValidateRejectsMissingNetwork(t *testing.T) {
	cfg := Defaults()
	agent := cfg.Agents["codex"]
	agent.Network = "proxy"
	cfg.Agents["codex"] = agent
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "missing network") {
		t.Fatalf("expected missing-network error, got %v", err)
	}
}

func TestSaveUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	if err := Save(path, Defaults()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config mode = %o, want 600", got)
	}
}

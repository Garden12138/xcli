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

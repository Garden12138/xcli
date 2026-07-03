package runtime

import (
	"reflect"
	"testing"

	"github.com/Garden12138/xcli/internal/config"
)

func TestBuildEnvironmentAppliesUnsetSetAndAgentOverrides(t *testing.T) {
	cfg := config.Defaults()
	cfg.Networks["direct"] = config.Network{
		Unset: []string{"HTTP_PROXY", "http_proxy"},
		Set:   map[string]string{"MODE": "network"},
	}
	agent := cfg.Agents["codex"]
	agent.Network = "direct"
	agent.Env = map[string]string{"MODE": "agent", "TOKEN": "secret"}

	got, err := BuildEnvironment([]string{"PATH=/bin", "HTTP_PROXY=http://proxy", "http_proxy=http://proxy", "MODE=parent"}, cfg, agent, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"MODE=agent", "PATH=/bin", "TOKEN=secret"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment = %#v, want %#v", got, want)
	}
}

func TestBuildEnvironmentRejectsUnknownOverride(t *testing.T) {
	cfg := config.Defaults()
	_, err := BuildEnvironment(nil, cfg, cfg.Agents["codex"], "missing")
	if err == nil {
		t.Fatal("expected unknown network error")
	}
}

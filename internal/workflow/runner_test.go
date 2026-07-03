//go:build darwin || linux
// +build darwin linux

package workflow

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/runstore"
)

func TestRunnerPassesExplicitStepOutput(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "fake-agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Agents["fake"] = config.AgentConfig{
		Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"}, Output: "text",
	}
	registry := agent.NewRegistry(cfg)
	setEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	runner := Runner{Config: cfg, Registry: registry, Store: store, Progress: io.Discard, Stderr: io.Discard}
	definition := Workflow{
		Version: 1, Name: "test", Cwd: ".",
		Steps: []Step{
			{ID: "one", Agent: "fake", Prompt: "alpha"},
			{ID: "two", Agent: "fake", DependsOn: []string{"one"}, Prompt: "got {{ steps.one.output }}"},
		},
	}
	execution, err := runner.Run(context.Background(), filepath.Join(directory, "workflow.yaml"), definition, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Status != "success" || len(execution.Steps) != 2 || execution.Steps[1].Output != "got alpha" {
		t.Fatalf("unexpected execution: %#v", execution)
	}
	if execution.Steps[0].OutputFile != "" {
		t.Fatal("temporary output path leaked into final execution")
	}
}

func TestRunnerStopsAndSkipsAfterFailure(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "fake-agent")
	data := "#!/bin/sh\nif [ \"$1\" = fail ]; then exit 7; fi\nprintf '%s' \"$1\"\n"
	if err := os.WriteFile(script, []byte(data), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Agents["fake"] = config.AgentConfig{Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"}, Output: "text"}
	setEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	store, _ := runstore.New()
	runner := Runner{Config: cfg, Registry: agent.NewRegistry(cfg), Store: store, Progress: io.Discard, Stderr: io.Discard}
	definition := Workflow{Version: 1, Name: "failure", Steps: []Step{
		{ID: "one", Agent: "fake", Prompt: "fail"},
		{ID: "two", Agent: "fake", DependsOn: []string{"one"}, Prompt: "never"},
	}}
	execution, err := runner.Run(context.Background(), filepath.Join(directory, "workflow.yaml"), definition, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if execution.ExitCode != 7 || execution.Steps[0].Status != "failed" || execution.Steps[1].Status != "skipped" {
		t.Fatalf("unexpected failure execution: %#v", execution)
	}
}

func TestValidateRejectsForwardDependency(t *testing.T) {
	cfg := config.Defaults()
	registry := agent.NewRegistry(cfg)
	definition := Workflow{Version: 1, Name: "invalid", Steps: []Step{
		{ID: "one", Agent: "codex", Prompt: "one", DependsOn: []string{"two"}},
		{ID: "two", Agent: "codex", Prompt: "two"},
	}}
	err := Validate(definition, registry, map[string]struct{}{})
	if err == nil || !strings.Contains(err.Error(), "declared earlier") {
		t.Fatalf("expected forward-dependency error, got %v", err)
	}
}

func setEnvironment(t *testing.T, key, value string) {
	old, existed := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

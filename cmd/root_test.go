//go:build darwin || linux
// +build darwin linux

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/workflow"
)

func TestRunCommandUsesDefaultGenericAgent(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "fake-agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.DefaultAgent = "fake"
	cfg.Agents["fake"] = config.AgentConfig{
		Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"}, Output: "text",
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))

	root := newRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--config", configPath, "run", "hello world", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v (stderr: %s)", err, stderr.String())
	}
	var result agent.RunResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), err)
	}
	if result.Agent != "fake" || result.Output != "hello world" || result.Status != "success" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRunPromptIsNotEvaluatedByShell(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "should-not-exist")
	script := filepath.Join(directory, "fake-agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.DefaultAgent = "fake"
	cfg.Agents["fake"] = config.AgentConfig{Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"}, Output: "text"}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))

	root := newRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "run", "hello; touch " + marker})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("prompt was evaluated as shell input; marker stat error = %v", err)
	}
}

func TestWorkflowMaxParallelFlagOverridesDefinition(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "fake-agent")
	scriptData := "#!/bin/sh\n" +
		"root=\"$XCLI_TEST_SYNC\"\n" +
		"touch \"$root/$1.started\"\n" +
		"if [ \"$1\" = first ]; then other=second; else other=first; fi\n" +
		"count=0\n" +
		"while [ ! -f \"$root/$other.started\" ]; do\n" +
		"  count=$((count + 1))\n" +
		"  if [ \"$count\" -gt 300 ]; then exit 9; fi\n" +
		"  sleep 0.01\n" +
		"done\n" +
		"printf '%s' \"$1\"\n"
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.Agents["fake"] = config.AgentConfig{
		Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"},
		Output: "text", Env: map[string]string{"XCLI_TEST_SYNC": directory},
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	workflowPath := filepath.Join(directory, "workflow.yaml")
	workflowData := "version: 1\n" +
		"name: cli-parallel\n" +
		"max_parallel: 1\n" +
		"steps:\n" +
		"  - id: first\n" +
		"    agent: fake\n" +
		"    prompt: first\n" +
		"  - id: second\n" +
		"    agent: fake\n" +
		"    prompt: second\n"
	if err := os.WriteFile(workflowPath, []byte(workflowData), 0o600); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))

	root := newRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{
		"--config", configPath, "workflow", "run", workflowPath,
		"--max-parallel", "2", "--json",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v (stderr: %s)", err, stderr.String())
	}
	var execution workflow.Execution
	if err := json.Unmarshal(stdout.Bytes(), &execution); err != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), err)
	}
	if execution.Status != "success" || execution.MaxParallel != 2 {
		t.Fatalf("unexpected execution: %#v", execution)
	}
}

func TestWorkflowRejectsInvalidMaxParallelOverride(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "fake-agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.Agents["fake"] = config.AgentConfig{
		Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"}, Output: "text",
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	workflowPath := filepath.Join(directory, "workflow.yaml")
	workflowData := "version: 1\nname: invalid-limit\nsteps:\n" +
		"  - id: one\n    agent: fake\n    prompt: one\n"
	if err := os.WriteFile(workflowPath, []byte(workflowData), 0o600); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))

	root := newRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"--config", configPath, "workflow", "run", workflowPath,
		"--max-parallel", "0",
	})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "greater than zero") {
		t.Fatalf("expected max-parallel error, got %v", err)
	}
}

func setCommandEnvironment(t *testing.T, key, value string) {
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

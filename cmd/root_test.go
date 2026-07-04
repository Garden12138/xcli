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
	"github.com/Garden12138/xcli/internal/routing"
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
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil || len(records) != 1 || records[0].Usage != nil {
		t.Fatalf("generic run should be untracked: %#v, %v", records, err)
	}
}

func TestRunBuiltinCapturesUsageWithoutLeakingJSONL(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "fake-codex")
	scriptData := "#!/bin/sh\n" +
		"case \" $* \" in\n" +
		"  *\" --json \"*) ;;\n" +
		"  *) echo 'structured mode missing' >&2; exit 9 ;;\n" +
		"esac\n" +
		"printf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"thread-usage\"}'\n" +
		"printf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"done\"}}'\n" +
		"printf '%s\\n' '{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":100,\"cached_input_tokens\":80,\"output_tokens\":10,\"reasoning_output_tokens\":4}}'\n"
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.DefaultAgent = "fake"
	cfg.Agents["fake"] = config.AgentConfig{Adapter: "codex", Command: script}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))

	root := newRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--config", configPath, "run", "plain output"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v (stderr: %s)", err, stderr.String())
	}
	if stdout.String() != "done\n" {
		t.Fatalf("plain stdout leaked structured events: %q", stdout.String())
	}
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil || len(records) != 1 {
		t.Fatalf("unexpected records: %#v, %v", records, err)
	}
	record := records[0]
	if record.SessionID != "thread-usage" || record.Usage == nil || record.Usage.TotalTokens != 110 {
		t.Fatalf("usage metadata was not recorded: %#v", record)
	}

	root = newRootCommand()
	stdout.Reset()
	stderr.Reset()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--config", configPath, "run", "--json", "json output"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute JSON: %v (stderr: %s)", err, stderr.String())
	}
	var result agent.RunResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), err)
	}
	if result.Output != "done" || result.Usage == nil || result.Usage.ReasoningTokens != 4 {
		t.Fatalf("unexpected JSON result: %#v", result)
	}
}

func TestRunAgentSelectionPrecedenceAndRecords(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "fake-agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.DefaultAgent = "default-agent"
	for _, name := range []string{"flag-agent", "positional-agent", "rule-agent", "default-agent"} {
		cfg.Agents[name] = config.AgentConfig{
			Adapter: "generic", Command: script, RunArgs: []string{name}, Output: "text",
		}
	}
	cfg.Routing.Rules = []config.RouteRule{{Name: "review", PromptRegex: "review", Agent: "rule-agent"}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "flag", args: []string{"--config", configPath, "run", "--agent", "flag-agent", "--json", "please review"}, want: "flag-agent"},
		{name: "positional", args: []string{"--config", configPath, "run", "positional-agent", "please review", "--json"}, want: "positional-agent"},
		{name: "rule", args: []string{"--config", configPath, "run", "please review", "--json"}, want: "rule-agent"},
		{name: "default", args: []string{"--config", configPath, "run", "implement this", "--json"}, want: "default-agent"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newRootCommand()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs(test.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("execute: %v (stderr: %s)", err, stderr.String())
			}
			var result agent.RunResult
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("decode output %q: %v", stdout.String(), err)
			}
			if result.Agent != test.want || result.Output != test.want {
				t.Fatalf("unexpected result: %#v", result)
			}
		})
	}

	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 {
		t.Fatalf("record count = %d, want 4", len(records))
	}
	wantSources := map[string]string{
		"flag-agent": routing.SourceFlag, "positional-agent": routing.SourcePositional,
		"rule-agent": routing.SourceRule, "default-agent": routing.SourceDefault,
	}
	for _, record := range records {
		if record.SelectionSource != wantSources[record.Agent] {
			t.Fatalf("unexpected selection metadata: %#v", record)
		}
		if record.Agent == "rule-agent" && record.RouteRule != "review" {
			t.Fatalf("routed record missing rule: %#v", record)
		}
		if record.Agent != "rule-agent" && record.RouteRule != "" {
			t.Fatalf("non-routed record has rule: %#v", record)
		}
	}
}

func TestRouteCommandExplainsWithoutRunningOrRecording(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "started")
	script := filepath.Join(directory, "fake-agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch \"$XCLI_ROUTE_MARKER\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.DefaultAgent = "codex"
	claude := cfg.Agents["claude"]
	claude.Command = script
	claude.Env = map[string]string{"XCLI_ROUTE_MARKER": marker}
	cfg.Agents["claude"] = claude
	cfg.Routing.Rules = []config.RouteRule{{Name: "review", PromptRegex: "review", Agent: "claude"}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(directory, "data")
	setCommandEnvironment(t, "XDG_DATA_HOME", dataPath)

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "route", "please review"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "Agent: claude\nSource: rule\nRule: review\n"; got != want {
		t.Fatalf("route output = %q, want %q", got, want)
	}

	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "route", "--json", "implement this"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var decision routing.Decision
	if err := json.Unmarshal(stdout.Bytes(), &decision); err != nil {
		t.Fatalf("decode route output %q: %v", stdout.String(), err)
	}
	if decision.Agent != "codex" || decision.Source != routing.SourceDefault || decision.Rule != "" {
		t.Fatalf("unexpected route decision: %#v", decision)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("route command started an agent; marker stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataPath, "xcli", "runs")); !os.IsNotExist(err) {
		t.Fatalf("route command created run records; runs stat error = %v", err)
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

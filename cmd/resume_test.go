//go:build darwin || linux
// +build darwin linux

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/routing"
	"github.com/Garden12138/xcli/internal/runstore"
	usagepkg "github.com/Garden12138/xcli/internal/usage"
)

func TestResumeNonInteractiveFromRunRecord(t *testing.T) {
	directory := t.TempDir()
	capturePath := filepath.Join(directory, "invocation.txt")
	script := filepath.Join(directory, "fake-codex")
	scriptData := "#!/bin/sh\n" +
		"{\n" +
		"  printf 'cwd=%s\\n' \"$PWD\"\n" +
		"  printf 'env=%s\\n' \"$RESUME_ENV\"\n" +
		"  for arg in \"$@\"; do printf 'arg=%s\\n' \"$arg\"; done\n" +
		"} > \"$RESUME_CAPTURE\"\n" +
		"printf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"new-session\"}'\n" +
		"printf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"continued\"}}'\n" +
		"printf '%s\\n' '{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":10,\"cached_input_tokens\":2,\"output_tokens\":4,\"reasoning_output_tokens\":1,\"total_tokens\":14}}'\n"
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.Agents["fake-codex"] = config.AgentConfig{
		Adapter: "codex", Command: script, Args: []string{"--base", "one"},
		Env: map[string]string{"RESUME_CAPTURE": capturePath, "RESUME_ENV": "present"},
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	source := runstore.Record{
		ID: "run-source", Kind: "run", Agent: "fake-codex", Cwd: directory,
		StartedAt: time.Now().UTC().Add(-time.Hour), EndedAt: time.Now().UTC().Add(-time.Hour),
		Status: "success", SessionID: "old-session",
	}
	if err := store.Save(source); err != nil {
		t.Fatal(err)
	}

	root := newRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{
		"--config", configPath, "resume", "run-source", "continue", "work", "--json",
		"--", "--sandbox", "workspace-write",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v (stderr: %s)", err, stderr.String())
	}
	var result agent.RunResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), err)
	}
	if result.Agent != "fake-codex" || result.Output != "continued" || result.SessionID != "new-session" || result.Usage == nil {
		t.Fatalf("unexpected result: %#v", result)
	}

	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatal(err)
	}
	physicalDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	wantInvocation := strings.Join([]string{
		"cwd=" + physicalDirectory,
		"env=present",
		"arg=--base", "arg=one", "arg=exec", "arg=resume", "arg=--json",
		"arg=--sandbox", "arg=workspace-write", "arg=old-session", "arg=continue work", "",
	}, "\n")
	if string(captured) != wantInvocation {
		t.Fatalf("invocation = %q, want %q", string(captured), wantInvocation)
	}

	records, err := store.List()
	if err != nil || len(records) != 2 {
		t.Fatalf("records = %#v, %v", records, err)
	}
	resumed := records[0]
	if resumed.Kind != "run" || resumed.Agent != "fake-codex" || resumed.SelectionSource != routing.SourceResume ||
		resumed.ResumedFrom != source.ID || resumed.ResumedStep != "" || resumed.SessionID != "new-session" || resumed.Usage == nil {
		t.Fatalf("unexpected resume record: %#v", resumed)
	}
	report, err := usagepkg.Build(records, usagepkg.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Totals.Tasks != 2 || report.Totals.TrackedTasks != 1 {
		t.Fatalf("resume was not aggregated as a task: %#v", report.Totals)
	}
}

func TestResumeInteractiveNativeSession(t *testing.T) {
	directory := t.TempDir()
	capturePath := filepath.Join(directory, "args.txt")
	script := filepath.Join(directory, "fake-opencode")
	scriptData := "#!/bin/sh\n" +
		"for arg in \"$@\"; do printf '%s\\n' \"$arg\"; done > \"$RESUME_CAPTURE\"\n" +
		"printf 'interactive-bytes'\n"
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.Agents["fake-opencode"] = config.AgentConfig{
		Adapter: "opencode", Command: script, Args: []string{"--base", "value"},
		Env: map[string]string{"RESUME_CAPTURE": capturePath},
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
	root.SetArgs([]string{
		"--config", configPath, "resume", "native-session", "--agent", "fake-opencode",
		"--", "--model", "provider/model",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v (stderr: %s)", err, stderr.String())
	}
	if stdout.String() != "interactive-bytes" {
		t.Fatalf("interactive stdout = %q", stdout.String())
	}
	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatal(err)
	}
	want := "--base\nvalue\n--model\nprovider/model\n--session\nnative-session\n"
	if string(captured) != want {
		t.Fatalf("args = %q, want %q", string(captured), want)
	}
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil || len(records) != 1 {
		t.Fatalf("records = %#v, %v", records, err)
	}
	if records[0].Kind != "use" || records[0].SessionID != "native-session" || records[0].Usage != nil || records[0].ResumedFrom != "" {
		t.Fatalf("unexpected interactive record: %#v", records[0])
	}
}

func TestResumeWorkflowStepRecordsParentAndCwdOverride(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "fake-gemini")
	scriptData := "#!/bin/sh\n" +
		"printf '%s\\n' '{\"type\":\"init\",\"session_id\":\"workflow-new-session\"}'\n" +
		"printf '%s\\n' '{\"type\":\"message\",\"role\":\"assistant\",\"content\":\"workflow continued\"}'\n"
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.Agents["fake-gemini"] = config.AgentConfig{Adapter: "gemini", Command: script}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	workflowRecord := runstore.Record{
		ID: "workflow-parent", Kind: "workflow", Cwd: filepath.Join(directory, "removed"),
		Steps: []runstore.StepRecord{{ID: "review", Agent: "fake-gemini", SessionID: "workflow-old-session"}},
	}
	if err := store.Save(workflowRecord); err != nil {
		t.Fatal(err)
	}

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"--config", configPath, "resume", workflowRecord.ID, "continue", "review",
		"--step", "review", "--cwd", directory, "--json",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var result agent.RunResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "workflow-new-session" || result.Output != "workflow continued" {
		t.Fatalf("unexpected result: %#v", result)
	}
	records, err := store.List()
	if err != nil || len(records) != 2 {
		t.Fatalf("records = %#v, %v", records, err)
	}
	resumed := records[0]
	if resumed.ResumedFrom != workflowRecord.ID || resumed.ResumedStep != "review" || resumed.Cwd != directory {
		t.Fatalf("unexpected workflow resume record: %#v", resumed)
	}
}

func TestResolveResumeTargetFromWorkflowAndNativeID(t *testing.T) {
	directory := t.TempDir()
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	workflowRecord := runstore.Record{
		ID: "workflow-source", Kind: "workflow", Cwd: directory,
		Steps: []runstore.StepRecord{
			{ID: "build", Agent: "codex", SessionID: "codex-session"},
			{ID: "review", Agent: "claude", SessionID: "claude-session"},
		},
	}
	if err := store.Save(workflowRecord); err != nil {
		t.Fatal(err)
	}
	target, err := resolveResumeTarget(store, workflowRecord.ID, "claude", "review")
	if err != nil {
		t.Fatal(err)
	}
	want := resumeTarget{
		Agent: "claude", SessionID: "claude-session", Cwd: directory,
		ResumedFrom: workflowRecord.ID, ResumedStep: "review",
	}
	if !reflect.DeepEqual(target, want) {
		t.Fatalf("target = %#v, want %#v", target, want)
	}
	raw, err := resolveResumeTarget(store, "external-session", "gemini", "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(raw, resumeTarget{Agent: "gemini", SessionID: "external-session"}) {
		t.Fatalf("raw target = %#v", raw)
	}
}

func TestResolveResumeTargetErrors(t *testing.T) {
	directory := t.TempDir()
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	records := []runstore.Record{
		{ID: "run-no-session", Kind: "run", Agent: "codex", Cwd: directory},
		{ID: "run-session", Kind: "run", Agent: "codex", Cwd: directory, SessionID: "session"},
		{ID: "workflow-source", Kind: "workflow", Cwd: directory, Steps: []runstore.StepRecord{{ID: "empty", Agent: "claude"}}},
	}
	for _, record := range records {
		if err := store.Save(record); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name   string
		value  string
		agent  string
		step   string
		needle string
	}{
		{name: "missing-record", value: "missing", needle: "pass --agent"},
		{name: "native-with-step", value: "missing", agent: "codex", step: "build", needle: "requires an existing workflow"},
		{name: "missing-session", value: "run-no-session", needle: "no recorded session id"},
		{name: "step-on-run", value: "run-session", step: "build", needle: "only be used with a workflow"},
		{name: "workflow-needs-step", value: "workflow-source", needle: "requires --step"},
		{name: "unknown-step", value: "workflow-source", step: "missing", needle: "has no step"},
		{name: "empty-step-session", value: "workflow-source", step: "empty", needle: "has no recorded session id"},
		{name: "agent-conflict", value: "run-session", agent: "claude", needle: "uses agent"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolveResumeTarget(store, test.value, test.agent, test.step)
			if err == nil || !strings.Contains(err.Error(), test.needle) {
				t.Fatalf("error = %v, want containing %q", err, test.needle)
			}
		})
	}
}

func TestResumeValidationErrors(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.Agents["custom"] = config.AgentConfig{
		Adapter: "generic", Command: "custom", RunArgs: []string{"{{ prompt }}"},
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(runstore.Record{ID: "generic-run", Kind: "run", Agent: "custom", Cwd: directory, SessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	missingCwd := filepath.Join(directory, "removed")
	if err := store.Save(runstore.Record{ID: "missing-cwd", Kind: "run", Agent: "codex", Cwd: missingCwd, SessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(runstore.Record{ID: "removed-agent", Kind: "run", Agent: "gone", Cwd: directory, SessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		args   []string
		needle string
	}{
		{name: "json-interactive", args: []string{"resume", "native", "--agent", "codex", "--json"}, needle: "--json requires a prompt"},
		{name: "generic", args: []string{"resume", "generic-run"}, needle: "does not support session resume"},
		{name: "removed-agent", args: []string{"resume", "removed-agent"}, needle: "unknown agent"},
		{name: "missing-cwd", args: []string{"resume", "missing-cwd"}, needle: "pass --cwd"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newRootCommand()
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			root.SetArgs(append([]string{"--config", configPath}, test.args...))
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), test.needle) {
				t.Fatalf("error = %v, want containing %q", err, test.needle)
			}
		})
	}
}

func TestResumeNonInteractivePreservesSessionAndExitCode(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "fake-claude")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' '{\"type\":\"result\",\"result\":\"partial\"}'\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.Agents["fake-claude"] = config.AgentConfig{Adapter: "claude", Command: script}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "resume", "original-session", "continue", "--agent", "fake-claude", "--json"})
	err := root.Execute()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 7 {
		t.Fatalf("error = %#v, want exit code 7", err)
	}
	var result agent.RunResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), err)
	}
	if result.SessionID != "original-session" || result.Output != "partial" || result.ExitCode != 7 || result.Status != "failed" {
		t.Fatalf("unexpected result: %#v", result)
	}
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil || len(records) != 1 || records[0].SessionID != "original-session" || records[0].ExitCode != 7 {
		t.Fatalf("records = %#v, %v", records, err)
	}
}

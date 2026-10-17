//go:build darwin || linux
// +build darwin linux

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
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
		{name: "legacy routing ignored", args: []string{"--config", configPath, "run", "please review", "--json"}, want: "default-agent"},
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
		"flag-agent": "flag", "positional-agent": "positional",
		"default-agent": "default",
	}
	for _, record := range records {
		if record.SelectionSource != wantSources[record.Agent] {
			t.Fatalf("unexpected selection metadata: %#v", record)
		}
		if record.Agent == "rule-agent" || record.RouteRule != "" {
			t.Fatalf("non-routed record has rule: %#v", record)
		}
	}
}

func TestMCPServeFindsProjectConfigurationFromChildDirectory(t *testing.T) {
	directory := t.TempDir()
	project := filepath.Join(directory, "project")
	child := filepath.Join(project, "src", "nested")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	server := filepath.Join(directory, "fake-project-mcp")
	if err := os.WriteFile(server, []byte("#!/bin/sh\nprintf '%s|' \"$PWD\"\ncat\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(project, ".xcli", "config.yaml")
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{"local": {Transport: "stdio", Command: server}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(child); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetIn(strings.NewReader("payload"))
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"mcp", "serve", "--project-config", ".xcli/config.yaml", "local"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	canonicalChild, _ := filepath.EvalSymlinks(child)
	if stdout.String() != canonicalChild+"|payload" {
		t.Fatalf("project MCP stdio/cwd changed: %q", stdout.String())
	}
}

func TestMCPServePassesThroughAndFiltersEnvironment(t *testing.T) {
	directory := t.TempDir()
	work := filepath.Join(directory, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(directory, "serve-state")
	server := filepath.Join(directory, "fake-mcp")
	script := "#!/bin/sh\n" +
		"printf '%s|%s|%s|%s' \"$PWD\" \"$SERVICE_TOKEN\" \"$LOG_LEVEL\" \"${OTHER_SECRET-unset}\" > \"$STATE_FILE\"\n" +
		"printf diagnostic >&2\n" +
		"cat\n"
	if err := os.WriteFile(server, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{
		"local": {
			Transport: "stdio", Command: server, Cwd: "work",
			Env:     map[string]string{"LOG_LEVEL": "debug", "STATE_FILE": statePath},
			EnvVars: []string{"SERVICE_TOKEN"},
		},
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "SERVICE_TOKEN", "secret")
	setCommandEnvironment(t, "OTHER_SECRET", "must-not-leak")
	dataPath := filepath.Join(directory, "data")
	setCommandEnvironment(t, "XDG_DATA_HOME", dataPath)
	payload := `{"jsonrpc":"2.0","id":1}`
	root := newRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetIn(strings.NewReader(payload))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--config", configPath, "mcp", "serve", "local"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != payload || stderr.String() != "diagnostic" {
		t.Fatalf("MCP stdio changed: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	resolvedWork, err := filepath.EvalSymlinks(work)
	if err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, statePath, resolvedWork+"|secret|debug|unset")
	if _, err := os.Stat(filepath.Join(dataPath, "xcli", "runs")); !os.IsNotExist(err) {
		t.Fatalf("MCP serve created run records: %v", err)
	}

	if err := os.Unsetenv("SERVICE_TOKEN"); err != nil {
		t.Fatal(err)
	}
	root = newRootCommand()
	root.SetArgs([]string{"--config", configPath, "mcp", "serve", "local"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "requires environment variable SERVICE_TOKEN") {
		t.Fatalf("expected missing variable error, got %v", err)
	}
}

func TestMCPServeForwardsCancellation(t *testing.T) {
	directory := t.TempDir()
	ready := filepath.Join(directory, "ready")
	interrupted := filepath.Join(directory, "interrupted")
	server := filepath.Join(directory, "waiting-mcp")
	script := "#!/bin/sh\n" +
		"trap 'printf interrupted > \"$INTERRUPTED\"; exit 0' INT\n" +
		"printf ready > \"$READY\"\n" +
		"while :; do sleep 0.05; done\n"
	if err := os.WriteFile(server, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{
		"waiting": {
			Transport: "stdio", Command: server,
			Env: map[string]string{"READY": ready, "INTERRUPTED": interrupted},
		},
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	root := newRootCommand()
	root.SetContext(ctx)
	root.SetIn(strings.NewReader(""))
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "mcp", "serve", "waiting"})
	finished := make(chan error, 1)
	go func() { finished <- root.Execute() }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("MCP server did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-finished:
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 130 {
			t.Fatalf("expected canceled exit code 130, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("MCP serve did not exit after cancellation")
	}
	assertFileContent(t, interrupted, "interrupted")
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
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

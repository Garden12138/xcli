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
	"github.com/Garden12138/xcli/internal/mcp"
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

func TestACPCommandPassesThroughStdioAndUsesSelectedAgent(t *testing.T) {
	directory := t.TempDir()
	workdir := filepath.Join(directory, "work")
	if err := os.Mkdir(workdir, 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(directory, "fake-acp")
	scriptData := "#!/bin/sh\n" +
		"printf '%s' \"$PWD\" > \"$XCLI_ACP_STATE/cwd\"\n" +
		"printf '%s' \"$XCLI_ACP_AGENT\" > \"$XCLI_ACP_STATE/agent\"\n" +
		"printf '%s' \"$XCLI_ACP_NETWORK\" > \"$XCLI_ACP_STATE/network\"\n" +
		"printf '%s\\n' \"$@\" > \"$XCLI_ACP_STATE/args\"\n" +
		"printf 'diagnostic' >&2\n" +
		"cat\n"
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.DefaultAgent = "default-acp"
	cfg.Networks["test"] = config.Network{Set: map[string]string{"XCLI_ACP_NETWORK": "direct"}}
	for _, name := range []string{"default-acp", "selected-acp"} {
		cfg.Agents[name] = config.AgentConfig{
			Adapter: "generic", Command: "unused", RunArgs: []string{"{{ prompt }}"}, Network: "test",
			Env: map[string]string{"XCLI_ACP_STATE": directory, "XCLI_ACP_AGENT": name},
			ACP: &config.ACPConfig{Command: script, Args: []string{"--stdio"}},
		}
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(directory, "data")
	setCommandEnvironment(t, "XDG_DATA_HOME", dataPath)
	payload := "{\"jsonrpc\":\"2.0\",\"id\":1}"

	run := func(agentName string, extra ...string) (string, string) {
		t.Helper()
		root := newRootCommand()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		root.SetIn(strings.NewReader(payload))
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		args := []string{"--config", configPath, "acp"}
		if agentName != "" {
			args = append(args, agentName)
		}
		args = append(args, "--cwd", workdir)
		if len(extra) > 0 {
			args = append(args, "--")
			args = append(args, extra...)
		}
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v (stderr: %s)", err, stderr.String())
		}
		return stdout.String(), stderr.String()
	}

	stdout, stderr := run("")
	if stdout != payload || stderr != "diagnostic" {
		t.Fatalf("stdio was not passed through exactly: stdout=%q stderr=%q", stdout, stderr)
	}
	assertFileContent(t, filepath.Join(directory, "agent"), "default-acp")

	stdout, stderr = run("selected-acp", "--debug", "value with spaces")
	if stdout != payload || stderr != "diagnostic" {
		t.Fatalf("selected stdio was not passed through exactly: stdout=%q stderr=%q", stdout, stderr)
	}
	assertFileContent(t, filepath.Join(directory, "agent"), "selected-acp")
	resolvedWorkdir, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, filepath.Join(directory, "cwd"), resolvedWorkdir)
	assertFileContent(t, filepath.Join(directory, "network"), "direct")
	assertFileContent(t, filepath.Join(directory, "args"), "--stdio\n--debug\nvalue with spaces\n")
	if _, err := os.Stat(filepath.Join(dataPath, "xcli", "runs")); !os.IsNotExist(err) {
		t.Fatalf("ACP command created run records; runs stat error = %v", err)
	}
}

func TestACPCommandErrorsAndExitCode(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.yaml")

	t.Run("generic requires ACP config", func(t *testing.T) {
		cfg := config.Defaults()
		cfg.DefaultAgent = "custom"
		cfg.Agents["custom"] = config.AgentConfig{Adapter: "generic", Command: "custom", RunArgs: []string{"{{ prompt }}"}}
		if err := config.Save(configPath, cfg); err != nil {
			t.Fatal(err)
		}
		root := newRootCommand()
		root.SetArgs([]string{"--config", configPath, "acp"})
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "configure agents.custom.acp") {
			t.Fatalf("expected unsupported error, got %v", err)
		}
	})

	t.Run("missing bridge has install hint", func(t *testing.T) {
		cfg := config.Defaults()
		cfg.DefaultAgent = "codex"
		if err := config.Save(configPath, cfg); err != nil {
			t.Fatal(err)
		}
		setCommandEnvironment(t, "PATH", directory)
		root := newRootCommand()
		root.SetArgs([]string{"--config", configPath, "acp"})
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "npm install -g @agentclientprotocol/codex-acp") {
			t.Fatalf("expected bridge install hint, got %v", err)
		}
	})

	t.Run("exit code is preserved", func(t *testing.T) {
		script := filepath.Join(directory, "failing-acp")
		if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 7\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		cfg := config.Defaults()
		cfg.DefaultAgent = "custom"
		cfg.Agents["custom"] = config.AgentConfig{
			Adapter: "generic", Command: "custom", RunArgs: []string{"{{ prompt }}"},
			ACP: &config.ACPConfig{Command: script},
		}
		if err := config.Save(configPath, cfg); err != nil {
			t.Fatal(err)
		}
		root := newRootCommand()
		root.SetArgs([]string{"--config", configPath, "acp"})
		err := root.Execute()
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 7 {
			t.Fatalf("expected exit code 7, got %v", err)
		}
	})
}

func TestACPCommandForwardsCancellation(t *testing.T) {
	directory := t.TempDir()
	ready := filepath.Join(directory, "ready")
	interrupted := filepath.Join(directory, "interrupted")
	script := filepath.Join(directory, "waiting-acp")
	scriptData := "#!/bin/sh\n" +
		"trap 'printf interrupted > \"$XCLI_ACP_INTERRUPTED\"; exit 0' INT\n" +
		"printf ready > \"$XCLI_ACP_READY\"\n" +
		"while :; do sleep 0.05; done\n"
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.DefaultAgent = "custom"
	cfg.Agents["custom"] = config.AgentConfig{
		Adapter: "generic", Command: "unused", RunArgs: []string{"{{ prompt }}"},
		Env: map[string]string{"XCLI_ACP_READY": ready, "XCLI_ACP_INTERRUPTED": interrupted},
		ACP: &config.ACPConfig{Command: script},
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
	root.SetArgs([]string{"--config", configPath, "acp"})
	finished := make(chan error, 1)
	go func() {
		finished <- root.Execute()
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("ACP child did not start")
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
		t.Fatal("ACP command did not exit after cancellation")
	}
	assertFileContent(t, interrupted, "interrupted")
}

func TestMCPPlanSyncAndIdempotence(t *testing.T) {
	directory := t.TempDir()
	home := filepath.Join(directory, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	nativeState := filepath.Join(directory, "native.json")
	if err := os.WriteFile(nativeState, []byte("[]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(directory, "commands.log")
	cli := filepath.Join(directory, "fake-codex")
	script := `#!/bin/sh
case "$1" in
  --version) echo 'codex-cli 1.0' ;;
  mcp)
    case "$2" in
      list) cat "$XCLI_NATIVE_MCP_STATE" ;;
      add)
        printf '%s\n' "$*" >> "$XCLI_NATIVE_MCP_LOG"
        printf '[{"name":"%s","transport":{"type":"streamable_http","url":"%s"}}]\n' "$3" "$5" > "$XCLI_NATIVE_MCP_STATE"
        ;;
      remove)
        printf '%s\n' "$*" >> "$XCLI_NATIVE_MCP_LOG"
        printf '[]\n' > "$XCLI_NATIVE_MCP_STATE"
        ;;
    esac
    ;;
esac
`
	if err := os.WriteFile(cli, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(directory, "xcli")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	codex := cfg.Agents["codex"]
	codex.Command = cli
	codex.Env = map[string]string{"XCLI_NATIVE_MCP_STATE": nativeState, "XCLI_NATIVE_MCP_LOG": logPath}
	cfg.Agents["codex"] = codex
	cfg.MCP.Servers = map[string]config.MCPServer{
		"docs": {Transport: "http", URL: "https://example.com/mcp", Targets: []string{"codex"}},
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "HOME", home)
	dataPath := filepath.Join(directory, "data")
	setCommandEnvironment(t, "XDG_DATA_HOME", dataPath)

	executeJSON := func(args ...string) (mcp.Plan, error) {
		t.Helper()
		root := newRootCommand()
		var stdout bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(append([]string{"--config", configPath}, args...))
		err := root.Execute()
		if err != nil {
			return mcp.Plan{}, err
		}
		var plan mcp.Plan
		if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
			t.Fatalf("decode MCP output %q: %v", stdout.String(), err)
		}
		return plan, nil
	}

	plan, err := executeJSON("mcp", "plan", "--target", "codex", "--launcher", launcher, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable || plan.Applied || len(plan.Changes) != 1 || plan.Changes[0].Action != mcp.ActionAdd {
		t.Fatalf("unexpected plan: %#v", plan)
	}

	root := newRootCommand()
	root.SetIn(strings.NewReader(""))
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "mcp", "sync", "--target", "codex", "--launcher", launcher})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "without a terminal") {
		t.Fatalf("expected non-interactive confirmation error, got %v", err)
	}

	plan, err = executeJSON("mcp", "sync", "--target", "codex", "--launcher", launcher, "--yes", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applied || plan.Changes[0].Status != mcp.StatusApplied {
		t.Fatalf("sync was not applied: %#v", plan)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(logData), "mcp add docs --url https://example.com/mcp") {
		t.Fatalf("unexpected native command log %q, %v", logData, err)
	}
	statePath := filepath.Join(dataPath, "xcli", "mcp-sync.json")
	info, err := os.Stat(statePath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("sync state mode = %v, %v", info.Mode().Perm(), err)
	}
	plan, err = executeJSON("mcp", "plan", "--target", "codex", "--launcher", launcher, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Action != mcp.ActionNoop {
		t.Fatalf("repeated plan is not idempotent: %#v", plan)
	}
	cfg.MCP.Servers["local"] = config.MCPServer{Transport: "stdio", Command: "server", Targets: []string{"codex"}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	root = newRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "mcp", "sync", "--target", "codex", "--yes"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "temporary xcli launcher") {
		t.Fatalf("expected temporary launcher error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataPath, "xcli", "runs")); !os.IsNotExist(err) {
		t.Fatalf("MCP sync created run records: %v", err)
	}
}

func TestProjectMCPPlanSyncWritesPortableCodexConfiguration(t *testing.T) {
	directory := t.TempDir()
	project := filepath.Join(directory, "project")
	configPath := filepath.Join(project, ".xcli", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(directory, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	cli := filepath.Join(bin, "fake-codex")
	launcherName := "xcli-portable"
	if err := os.WriteFile(cli, []byte("#!/bin/sh\n[ \"$1\" = --version ] && echo 'codex 1.0'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, launcherName), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	codex := cfg.Agents["codex"]
	codex.Command = cli
	cfg.Agents["codex"] = codex
	cfg.MCP.Servers = map[string]config.MCPServer{
		"tools": {Transport: "stdio", Command: "server", EnvVars: []string{"TOKEN"}, Targets: []string{"codex"}},
		"docs":  {Transport: "http", URL: "https://example.com/mcp", Targets: []string{"codex"}},
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))

	execute := func(arguments ...string) (mcp.Plan, error) {
		t.Helper()
		root := newRootCommand()
		var stdout bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(append([]string{"--config", configPath, "mcp"}, arguments...))
		err := root.Execute()
		if err != nil {
			return mcp.Plan{}, err
		}
		var plan mcp.Plan
		if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
			t.Fatalf("decode project MCP plan %q: %v", stdout.String(), err)
		}
		return plan, nil
	}

	plan, err := execute("plan", "--scope", "project", "--project", project, "--target", "codex", "--launcher", launcherName, "--json")
	if err != nil {
		t.Fatal(err)
	}
	canonicalProject, _ := filepath.EvalSymlinks(project)
	if plan.Scope != mcp.ScopeProject || plan.ProjectDir != canonicalProject || plan.Launcher != launcherName || len(plan.Changes) != 2 {
		t.Fatalf("unexpected project plan: %#v", plan)
	}
	plan, err = execute("sync", "--scope", "project", "--project", project, "--target", "codex", "--launcher", launcherName, "--yes", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applied {
		t.Fatalf("project plan was not applied: %#v", plan)
	}
	projectCodex := filepath.Join(project, ".codex", "config.toml")
	data, err := os.ReadFile(projectCodex)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, value := range []string{
		`command = "xcli-portable"`,
		`args = ["mcp","serve","--project-config",".xcli/config.yaml","tools"]`,
		`url = "https://example.com/mcp"`,
	} {
		if !strings.Contains(text, value) {
			t.Fatalf("project Codex config %q missing %q", text, value)
		}
	}
	if strings.Contains(text, canonicalProject) || strings.Contains(text, configPath) {
		t.Fatalf("project configuration contains a machine-specific path: %s", text)
	}
	info, err := os.Stat(projectCodex)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("new project config mode = %v, %v", info.Mode().Perm(), err)
	}
	plan, err = execute("plan", "--scope", "project", "--project", project, "--target", "codex", "--launcher", launcherName, "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range plan.Changes {
		if change.Action != mcp.ActionNoop {
			t.Fatalf("repeated project plan is not idempotent: %#v", plan)
		}
	}
}

func TestProjectMCPValidation(t *testing.T) {
	directory := t.TempDir()
	project := filepath.Join(directory, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideConfig := filepath.Join(directory, "config.yaml")
	if err := config.Save(outsideConfig, config.Defaults()); err != nil {
		t.Fatal(err)
	}
	insideConfig := filepath.Join(project, ".xcli", "config.yaml")
	if err := config.Save(insideConfig, config.Defaults()); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing project", args: []string{"--config", outsideConfig, "mcp", "plan", "--scope", "project"}, want: "--project is required"},
		{name: "project with user scope", args: []string{"--config", outsideConfig, "mcp", "plan", "--project", project}, want: "only valid with --scope project"},
		{name: "source outside project", args: []string{"--config", outsideConfig, "mcp", "plan", "--scope", "project", "--project", project}, want: "outside project"},
		{name: "absolute launcher", args: []string{"--config", insideConfig, "mcp", "plan", "--scope", "project", "--project", project, "--launcher", "/usr/bin/xcli"}, want: "PATH command name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newRootCommand()
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			root.SetArgs(test.args)
			if err := root.Execute(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
		})
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

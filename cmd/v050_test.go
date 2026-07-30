//go:build darwin || linux
// +build darwin linux

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/jobs"
	"github.com/Garden12138/xcli/internal/mcp"
	"github.com/Garden12138/xcli/internal/runstore"
	"github.com/Garden12138/xcli/internal/workflow"
	"github.com/Garden12138/xcli/internal/worktree"
)

func TestWorkflowPlanReportsAvailableAndMissingCommands(t *testing.T) {
	directory := t.TempDir()
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	configPath := filepath.Join(directory, "config.yaml")
	if err := config.Save(configPath, config.Defaults()); err != nil {
		t.Fatal(err)
	}
	workflowPath := filepath.Join(directory, "workflow.yaml")
	data := "version: 1\nname: preflight\nvars:\n  value: default\nsteps:\n" +
		"  - id: available\n    command: printf\n    args: ['{{ vars.value }}']\n" +
		"  - id: missing\n    command: ./missing-command\n    depends_on: [available]\n"
	if err := os.WriteFile(workflowPath, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "workflow", "plan", workflowPath, "--var", "value=planned", "--json"})
	err := root.Execute()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("expected preflight exit 1, got %v", err)
	}
	var plan workflow.Plan
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatalf("decode plan %q: %v", stdout.String(), err)
	}
	if plan.Applicable || len(plan.Steps) != 2 ||
		plan.Steps[0].Availability != workflow.AvailabilityOK ||
		plan.Steps[1].Availability != workflow.AvailabilityMissing ||
		len(plan.Variables) != 1 || plan.Variables[0] != "value" {
		t.Fatalf("unexpected workflow plan: %#v", plan)
	}
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil || len(records) != 0 {
		t.Fatalf("workflow plan created run records: %#v, %v", records, err)
	}
}

func TestMCPCheckExitCodesForCleanDriftAndConflict(t *testing.T) {
	directory := t.TempDir()
	home := filepath.Join(directory, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "HOME", home)
	setCommandEnvironment(t, "XDG_CONFIG_HOME", filepath.Join(directory, "config"))
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	fakeClaude := filepath.Join(directory, "claude")
	if err := os.WriteFile(fakeClaude, []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf 'claude 1.2.3'; fi\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "xcli.yaml")
	cfg := config.Defaults()
	cfg.Agents["claude"] = config.AgentConfig{Adapter: "claude", Command: fakeClaude}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	plan, err := executeMCPCheck(t, configPath)
	if err != nil || !plan.InSync || !plan.Applicable {
		t.Fatalf("clean check = %#v, %v", plan, err)
	}

	cfg.MCP.Servers = map[string]config.MCPServer{
		"docs": {Transport: "http", URL: "https://desired.example/mcp", Targets: []string{"claude"}},
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	plan, err = executeMCPCheck(t, configPath)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 || plan.InSync ||
		len(plan.Changes) != 1 || plan.Changes[0].Action != mcp.ActionAdd {
		t.Fatalf("drift check = %#v, %v", plan, err)
	}

	native := `{"mcpServers":{"docs":{"type":"http","url":"https://native.example/mcp"}}}`
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(native), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err = executeMCPCheck(t, configPath)
	if !errors.As(err, &exitErr) || exitErr.Code != 2 || plan.Applicable ||
		len(plan.Changes) != 1 || plan.Changes[0].Action != mcp.ActionConflict {
		t.Fatalf("conflict check = %#v, %v", plan, err)
	}
}

func TestDoctorDeepReportsCapabilitiesAndACPAvailability(t *testing.T) {
	directory := t.TempDir()
	fakeAgent := filepath.Join(directory, "fake-agent")
	if err := os.WriteFile(fakeAgent, []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf 'fake 2.0'; fi\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.Agents = map[string]config.AgentConfig{
		"fake": {
			Adapter: "generic", Command: fakeAgent, RunArgs: []string{"{{ prompt }}"}, Output: "jsonl",
			ACP: &config.ACPConfig{Command: fakeAgent},
		},
	}
	cfg.DefaultAgent = "fake"
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "doctor", "fake", "--deep", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	item := report.Agents["fake"]
	if !report.Deep || item.Version != "fake 2.0" || item.Capabilities == nil ||
		!item.Capabilities.Run || !item.Capabilities.StructuredOutput || !item.Capabilities.ACP ||
		item.ACPAvailable == nil || !*item.ACPAvailable {
		t.Fatalf("unexpected doctor report: %#v", report)
	}

	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "doctor", "fake", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var legacyShape map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &legacyShape); err != nil {
		t.Fatal(err)
	}
	if _, exists := legacyShape["deep"]; exists {
		t.Fatalf("basic doctor JSON gained deep-only fields: %s", stdout.String())
	}
	agents, ok := legacyShape["agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected basic doctor JSON: %s", stdout.String())
	}
	fake, ok := agents["fake"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected basic doctor agent JSON: %s", stdout.String())
	}
	if _, exists := fake["capabilities"]; exists {
		t.Fatalf("basic doctor JSON gained capabilities: %s", stdout.String())
	}
	if _, exists := fake["acp_available"]; exists {
		t.Fatalf("basic doctor JSON gained ACP state: %s", stdout.String())
	}
}

func TestRunWorktreeRecordsIsolationMetadata(t *testing.T) {
	directory := t.TempDir()
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	repository := filepath.Join(directory, "repo")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	runCommandGit(t, repository, "init")
	runCommandGit(t, repository, "config", "user.name", "xcli tests")
	runCommandGit(t, repository, "config", "user.email", "xcli@example.invalid")
	if err := os.WriteFile(filepath.Join(repository, "tracked.txt"), []byte("tracked"), 0o600); err != nil {
		t.Fatal(err)
	}
	runCommandGit(t, repository, "add", ".")
	runCommandGit(t, repository, "commit", "-m", "initial")

	fakeAgent := filepath.Join(directory, "fake-agent")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf 'fake 3.0'; exit 0; fi\nprintf '%s' \"$1\"\n"
	if err := os.WriteFile(fakeAgent, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.DefaultAgent = "fake"
	cfg.Agents["fake"] = config.AgentConfig{
		Adapter: "generic", Command: fakeAgent, RunArgs: []string{"{{ prompt }}"}, Output: "text",
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "run", "--cwd", repository, "--worktree", "--json", "isolated"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var result agent.RunResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Output != "isolated" {
		t.Fatalf("unexpected run result: %#v", result)
	}
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil || len(records) != 1 {
		t.Fatalf("records = %#v, %v", records, err)
	}
	record := records[0]
	if record.WorkspaceMode != "worktree" || record.Repository == "" || record.WorktreePath == "" ||
		record.WorktreeBranch == "" || record.BaseCommit == "" || record.AgentVersion != "fake 3.0" ||
		!strings.HasPrefix(record.Cwd, record.WorktreePath) {
		t.Fatalf("missing isolation metadata: %#v", record)
	}
	manager, err := worktree.New()
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := manager.Load(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(metadata, false); err != nil {
		t.Fatal(err)
	}
}

func TestDetachedRunUsesTheRecordedWorktree(t *testing.T) {
	installDetachedTestHelper(t)
	directory := t.TempDir()
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	repository := filepath.Join(directory, "repo")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	runCommandGit(t, repository, "init")
	runCommandGit(t, repository, "config", "user.name", "xcli tests")
	runCommandGit(t, repository, "config", "user.email", "xcli@example.invalid")
	if err := os.WriteFile(filepath.Join(repository, "tracked.txt"), []byte("tracked"), 0o600); err != nil {
		t.Fatal(err)
	}
	runCommandGit(t, repository, "add", ".")
	runCommandGit(t, repository, "commit", "-m", "initial")

	fakeAgent := filepath.Join(directory, "fake-agent")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf 'fake 4.0'; exit 0; fi\nprintf '%s' \"$PWD\"\n"
	if err := os.WriteFile(fakeAgent, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.DefaultAgent = "fake"
	cfg.Agents["fake"] = config.AgentConfig{
		Adapter: "generic", Command: fakeAgent, RunArgs: []string{"{{ prompt }}"}, Output: "text",
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"--config", configPath, "run", "--cwd", repository, "--worktree", "--detach", "--json", "detached",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var launched jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &launched); err != nil {
		t.Fatal(err)
	}
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	record := waitForJob(t, store, launched.ID, "success")
	if !record.Background || record.WorkspaceMode != "worktree" ||
		record.Cwd != record.WorktreePath || record.AgentVersion != "fake 4.0" {
		t.Fatalf("unexpected detached worktree record: %#v", record)
	}
	manager, err := worktree.New()
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := manager.Load(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(metadata, false); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowRunUsesOneWorktreeForCommandSteps(t *testing.T) {
	directory := t.TempDir()
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	repository := filepath.Join(directory, "repo")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	runCommandGit(t, repository, "init")
	runCommandGit(t, repository, "config", "user.name", "xcli tests")
	runCommandGit(t, repository, "config", "user.email", "xcli@example.invalid")
	workflowPath := filepath.Join(repository, "workflow.yaml")
	workflowData := "version: 1\nname: isolated-command\nsteps:\n  - id: where\n    command: pwd\n"
	if err := os.WriteFile(workflowPath, []byte(workflowData), 0o600); err != nil {
		t.Fatal(err)
	}
	runCommandGit(t, repository, "add", ".")
	runCommandGit(t, repository, "commit", "-m", "initial")
	configPath := filepath.Join(directory, "config.yaml")
	if err := config.Save(configPath, config.Defaults()); err != nil {
		t.Fatal(err)
	}

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "workflow", "run", workflowPath, "--worktree", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var execution workflow.Execution
	if err := json.Unmarshal(stdout.Bytes(), &execution); err != nil {
		t.Fatal(err)
	}
	if execution.Status != "success" || len(execution.Steps) != 1 ||
		execution.Steps[0].Output != execution.Cwd {
		t.Fatalf("unexpected isolated workflow: %#v", execution)
	}
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load(execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.WorkspaceMode != "worktree" || record.Cwd != execution.Cwd ||
		record.WorktreePath != execution.Cwd || record.WorktreeBranch == "" {
		t.Fatalf("unexpected isolated workflow record: %#v", record)
	}
	manager, err := worktree.New()
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := manager.Load(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(metadata, false); err != nil {
		t.Fatal(err)
	}
}

func executeMCPCheck(t *testing.T, configPath string) (mcp.Plan, error) {
	t.Helper()
	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "mcp", "check", "--target", "claude", "--json"})
	err := root.Execute()
	var plan mcp.Plan
	if decodeErr := json.Unmarshal(stdout.Bytes(), &plan); decodeErr != nil {
		t.Fatalf("decode MCP check %q: %v", stdout.String(), decodeErr)
	}
	return plan, err
}

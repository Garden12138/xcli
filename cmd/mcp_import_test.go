package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/mcp"
)

func TestMCPImportPlanApplyAndSyncRoundTrip(t *testing.T) {
	directory := t.TempDir()
	home := filepath.Join(directory, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "HOME", home)
	setCommandEnvironment(t, "XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	native := `{
  "mcpServers": {
    "tools": {"type":"stdio","command":"npx","args":["-y","tools"],"env":{"TOKEN":"${TOKEN}"}},
    "secret": {"type":"stdio","command":"server","env":{"TOKEN":"do-not-copy-this"}}
  }
}
`
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(native), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(directory, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeClaude := filepath.Join(bin, "claude")
	if err := os.WriteFile(fakeClaude, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(bin, "xcli")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	claude := cfg.Agents["claude"]
	claude.Command = fakeClaude
	cfg.Agents["claude"] = claude
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	executeImport := func(arguments ...string) (mcp.ImportPlan, error) {
		t.Helper()
		root := newRootCommand()
		var stdout bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(append([]string{"--config", configPath, "mcp", "import"}, arguments...))
		err := root.Execute()
		if err != nil {
			return mcp.ImportPlan{}, err
		}
		var plan mcp.ImportPlan
		if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
			t.Fatalf("decode import plan %q: %v", stdout.String(), err)
		}
		return plan, nil
	}
	plan, err := executeImport("plan", "--target", "claude", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable || len(plan.Changes) != 2 || plan.Changes[0].Action != mcp.ImportActionUnsupported || plan.Changes[1].Action != mcp.ImportActionAdd {
		t.Fatalf("unexpected import plan: %#v", plan)
	}
	encoded, _ := json.Marshal(plan)
	if strings.Contains(string(encoded), "do-not-copy-this") {
		t.Fatalf("import plan leaked native environment value: %s", encoded)
	}
	root := newRootCommand()
	var textPlan bytes.Buffer
	root.SetOut(&textPlan)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "mcp", "import", "plan", "--target", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(textPlan.String(), "SERVER") || !strings.Contains(textPlan.String(), "TARGETS") || !strings.Contains(textPlan.String(), "ACTION") || !strings.Contains(textPlan.String(), "STATUS") || !strings.Contains(textPlan.String(), "DETAIL") {
		t.Fatalf("import text table header changed: %q", textPlan.String())
	}

	root = newRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "mcp", "import", "apply", "--target", "claude", "--json"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "--json requires --yes") {
		t.Fatalf("expected JSON confirmation error, got %v", err)
	}

	root = newRootCommand()
	root.SetIn(strings.NewReader(""))
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "mcp", "import", "apply", "--target", "claude"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "without a terminal") {
		t.Fatalf("expected import confirmation protection, got %v", err)
	}
	plan, err = executeImport("apply", "--target", "claude", "--yes", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applied || plan.Changes[1].Status != mcp.StatusApplied {
		t.Fatalf("import was not applied: %#v", plan)
	}
	loaded, _, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	tools := loaded.MCP.Servers["tools"]
	if tools.Command != "npx" || len(tools.EnvVars) != 1 || strings.Join(tools.Targets, ",") != "claude" {
		t.Fatalf("unexpected imported xcli server: %#v", tools)
	}
	if _, exists := loaded.MCP.Servers["secret"]; exists {
		t.Fatal("unsupported server was imported")
	}
	repeated, err := executeImport("plan", "--target", "claude", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Changes[0].Action != mcp.ImportActionUnsupported || repeated.Changes[1].Action != mcp.ImportActionNoop {
		t.Fatalf("repeated import was not idempotent: %#v", repeated)
	}

	root = newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "mcp", "plan", "--target", "claude", "--launcher", launcher, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var syncPlan mcp.Plan
	if err := json.Unmarshal(stdout.Bytes(), &syncPlan); err != nil {
		t.Fatal(err)
	}
	if len(syncPlan.Changes) != 1 || syncPlan.Changes[0].Action != mcp.ActionUpdate {
		t.Fatalf("imported stdio entry did not round-trip into a launcher update: %#v", syncPlan)
	}
}

func TestMCPImportProjectDiscoversExistingFilesWithoutInstalledCLI(t *testing.T) {
	directory := t.TempDir()
	project := filepath.Join(directory, "project")
	configPath := filepath.Join(project, ".xcli", "config.yaml")
	if err := config.Save(configPath, config.Defaults()); err != nil {
		t.Fatal(err)
	}
	nativePath := filepath.Join(project, ".mcp.json")
	if err := os.WriteFile(nativePath, []byte(`{"mcpServers":{"docs":{"type":"http","url":"https://example.com/mcp"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "mcp", "import", "plan", "--scope", "project", "--project", project, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var plan mcp.ImportPlan
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Scope != mcp.ScopeProject || len(plan.Targets) != 1 || plan.Targets[0] != "claude" || len(plan.Changes) != 1 || plan.Changes[0].Action != mcp.ImportActionAdd {
		t.Fatalf("unexpected discovered project import: %#v", plan)
	}

	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "mcp", "import", "plan", "--scope", "project", "--project", project, "--target", "codex", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 1 || plan.Targets[0] != "codex" || len(plan.Changes) != 0 {
		t.Fatalf("explicit missing target was not treated as empty: %#v", plan)
	}
}

func TestMCPImportApplyCreatesMissingUserSource(t *testing.T) {
	directory := t.TempDir()
	home := filepath.Join(directory, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "HOME", home)
	setCommandEnvironment(t, "XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"mcpServers":{"docs":{"type":"http","url":"https://example.com/mcp"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "new", "config.yaml")
	root := newRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "mcp", "import", "apply", "--target", "claude", "--yes", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version: 1") || !strings.Contains(string(data), "docs:") {
		t.Fatalf("missing source was not created minimally: %s", data)
	}
	info, err := os.Stat(configPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("new user source mode = %v, %v", info.Mode().Perm(), err)
	}
}

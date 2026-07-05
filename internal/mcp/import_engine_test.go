package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/config"
)

func nativeCandidate(target string, server config.MCPServer, entry Entry) NativeCandidate {
	return NativeCandidate{Name: "tools", Target: target, Server: server, Fingerprint: entry}
}

func TestBuildImportPlanMergesEquivalentTargetsAndClaimsOwnership(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "config.yaml")
	server := config.MCPServer{Transport: "stdio", Command: "npx", Args: []string{"-y", "tools"}, EnvVars: []string{"TOKEN"}}
	entry := Entry{Transport: "stdio", Command: "npx", Args: []string{"-y", "tools"}, EnvVars: []string{"TOKEN"}}
	snapshots := map[string]NativeSnapshot{
		"claude": {Entries: map[string]NativeCandidate{"tools": nativeCandidate("claude", server, entry)}},
		"codex":  {Entries: map[string]NativeCandidate{"tools": nativeCandidate("codex", server, entry)}},
	}
	cfg := config.Defaults()
	state := newState(filepath.Join(directory, "state.json"))
	plan, err := BuildImportPlan(cfg, snapshots, state, ImportBuildOptions{SourceConfig: source, Scope: ScopeUser, Targets: []string{"codex", "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable || len(plan.Changes) != 1 || plan.Changes[0].Action != ImportActionAdd || strings.Join(plan.Changes[0].Targets, ",") != "claude,codex" {
		t.Fatalf("unexpected merged import plan: %#v", plan)
	}
	if len(plan.Changes[0].desired.Targets) != 2 || len(plan.Changes[0].ownership) != 2 {
		t.Fatalf("missing desired target or ownership aggregation: %#v", plan.Changes[0])
	}
}

func TestBuildImportPlanConflictsAndForcePreservesCoverage(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{
		"tools": {Transport: "stdio", Command: "old", Targets: []string{"claude"}},
	}
	server := config.MCPServer{Transport: "stdio", Command: "new"}
	entry := Entry{Transport: "stdio", Command: "new"}
	snapshots := map[string]NativeSnapshot{"codex": {Entries: map[string]NativeCandidate{"tools": nativeCandidate("codex", server, entry)}}}
	state := newState(filepath.Join(directory, "state.json"))
	options := ImportBuildOptions{SourceConfig: source, Scope: ScopeUser, Targets: []string{"codex"}}
	plan, err := BuildImportPlan(cfg, snapshots, state, options)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable || plan.Changes[0].Action != ImportActionConflict {
		t.Fatalf("expected source conflict: %#v", plan)
	}
	options.Force = true
	plan, err = BuildImportPlan(cfg, snapshots, state, options)
	if err != nil {
		t.Fatal(err)
	}
	change := plan.Changes[0]
	if !plan.Applicable || change.Action != ImportActionUpdate || strings.Join(change.desired.Targets, ",") != "claude,codex" || !strings.Contains(change.Detail, "preserving target coverage") {
		t.Fatalf("unexpected forced update: %#v", plan)
	}
}

func TestBuildImportPlanExtendsEquivalentTargetCoverage(t *testing.T) {
	cfg := config.Defaults()
	server := config.MCPServer{Transport: "http", URL: "https://example.com/mcp", Targets: []string{"claude"}}
	cfg.MCP.Servers = map[string]config.MCPServer{"docs": server}
	native := NativeCandidate{Name: "docs", Target: "codex", Server: config.MCPServer{Transport: "http", URL: server.URL}, Fingerprint: Entry{Transport: "http", URL: server.URL}}
	plan, err := BuildImportPlan(cfg, map[string]NativeSnapshot{"codex": {Entries: map[string]NativeCandidate{"docs": native}}}, newState(""), ImportBuildOptions{
		SourceConfig: "/config", Scope: ScopeUser, Targets: []string{"codex"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Action != ImportActionUpdate || strings.Join(plan.Changes[0].desired.Targets, ",") != "claude,codex" {
		t.Fatalf("equivalent target coverage was not extended: %#v", plan)
	}
}

func TestBuildImportPlanForceAcceptsOwnershipDrift(t *testing.T) {
	source := "/config"
	server := config.MCPServer{Transport: "http", URL: "https://example.com/mcp", Targets: []string{"codex"}}
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{"docs": server}
	entry := Entry{Transport: "http", URL: server.URL}
	native := NativeCandidate{Name: "docs", Target: "codex", Server: config.MCPServer{Transport: "http", URL: server.URL}, Fingerprint: entry}
	state := newState("")
	state.Set("codex", "docs", Ownership{SourceConfig: source, Fingerprint: "stale"})
	options := ImportBuildOptions{SourceConfig: source, Scope: ScopeUser, Targets: []string{"codex"}}
	plan, err := BuildImportPlan(cfg, map[string]NativeSnapshot{"codex": {Entries: map[string]NativeCandidate{"docs": native}}}, state, options)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable || plan.Changes[0].Action != ImportActionConflict {
		t.Fatalf("ownership drift did not conflict: %#v", plan)
	}
	options.Force = true
	plan, err = BuildImportPlan(cfg, map[string]NativeSnapshot{"codex": {Entries: map[string]NativeCandidate{"docs": native}}}, state, options)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable || plan.Changes[0].Action != ImportActionClaim || !strings.Contains(plan.Changes[0].Detail, "accepted native ownership") {
		t.Fatalf("force did not accept ownership drift: %#v", plan)
	}
}

func TestBuildImportPlanRejectsDivergentNativeDefinitionsEvenWithForce(t *testing.T) {
	one := config.MCPServer{Transport: "http", URL: "https://one.example/mcp"}
	two := config.MCPServer{Transport: "http", URL: "https://two.example/mcp"}
	snapshots := map[string]NativeSnapshot{
		"claude": {Entries: map[string]NativeCandidate{"tools": nativeCandidate("claude", one, Entry{Transport: "http", URL: one.URL})}},
		"codex":  {Entries: map[string]NativeCandidate{"tools": nativeCandidate("codex", two, Entry{Transport: "http", URL: two.URL})}},
	}
	plan, err := BuildImportPlan(config.Defaults(), snapshots, newState(""), ImportBuildOptions{SourceConfig: "/config", Scope: ScopeUser, Targets: []string{"claude", "codex"}, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable || plan.Changes[0].Action != ImportActionConflict || !strings.Contains(plan.Changes[0].Detail, "differently") {
		t.Fatalf("divergent definitions were not rejected: %#v", plan)
	}
}

func TestUnsupportedImportDoesNotBlockSafeServer(t *testing.T) {
	safe := config.MCPServer{Transport: "http", URL: "https://example.com/mcp"}
	snapshot := NativeSnapshot{Entries: map[string]NativeCandidate{
		"docs":   {Name: "docs", Target: "claude", Server: safe, Fingerprint: Entry{Transport: "http", URL: safe.URL}},
		"secret": {Name: "secret", Target: "claude", Unsupported: "contains static environment values"},
	}}
	plan, err := BuildImportPlan(config.Defaults(), map[string]NativeSnapshot{"claude": snapshot}, newState(""), ImportBuildOptions{SourceConfig: "/config", Scope: ScopeUser, Targets: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable || len(plan.Changes) != 2 || plan.Changes[0].Action != ImportActionAdd || plan.Changes[1].Action != ImportActionUnsupported {
		t.Fatalf("unexpected safe/unsupported plan: %#v", plan)
	}
}

func TestApplyImportPlanPreservesYAMLAndClaimsOwnership(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "config.yaml")
	original := "# root comment\nversion: 1\ndefault_agent: codex\n\nmcp:\n  # server comment\n  servers:\n    existing:\n      transport: http\n      url: https://old.example/mcp\n"
	if err := os.WriteFile(source, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	desired := config.MCPServer{Transport: "http", URL: "https://example.com/mcp", Targets: []string{"codex"}}
	entry := Entry{Transport: "http", URL: desired.URL}
	plan := ImportPlan{SourceConfig: source, Scope: ScopeUser, Applicable: true, Changes: []ImportChange{{
		Server: "docs", Targets: []string{"codex"}, Action: ImportActionAdd, Status: StatusPlanned,
		desired: &desired, ownership: map[string]Entry{"codex": entry},
	}}}
	state := newState(filepath.Join(directory, "state.json"))
	if err := ApplyImportPlan(&plan, state); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "# root comment") || !strings.Contains(text, "# server comment") || !strings.Contains(text, "default_agent: codex") || !strings.Contains(text, "existing:") || !strings.Contains(text, "docs:") {
		t.Fatalf("YAML patch lost comments or unrelated configuration: %s", text)
	}
	info, err := os.Stat(source)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("source mode = %v, %v", info.Mode().Perm(), err)
	}
	owner, ok := state.Get("codex", "docs")
	if !ok || owner.SourceConfig != source || owner.Fingerprint != entry.Fingerprint() || !plan.Applied || plan.Changes[0].Status != StatusApplied {
		t.Fatalf("ownership or status not applied: %#v %#v", owner, plan)
	}
}

func TestApplyImportPlanCreatesMinimalUserConfiguration(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "nested", "config.yaml")
	desired := config.MCPServer{Transport: "http", URL: "https://example.com/mcp", Targets: []string{"claude"}}
	plan := ImportPlan{SourceConfig: source, Scope: ScopeUser, Applicable: true, Changes: []ImportChange{{
		Server: "docs", Targets: []string{"claude"}, Action: ImportActionAdd, Status: StatusPlanned,
		desired: &desired, ownership: map[string]Entry{"claude": {Transport: "http", URL: desired.URL}},
	}}}
	state := newState(filepath.Join(directory, "state.json"))
	if err := ApplyImportPlan(&plan, state); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version: 1") || !strings.Contains(string(data), "mcp:") || strings.Contains(string(data), "agents:") {
		t.Fatalf("unexpected minimal source: %s", data)
	}
	info, err := os.Stat(source)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("new source mode = %v, %v", info.Mode().Perm(), err)
	}
}

func TestGeneratedWrapperForCurrentSourceIsClaimedWithoutRecursion(t *testing.T) {
	source := "/tmp/xcli-config.yaml"
	server := config.MCPServer{Transport: "stdio", Command: "npx", Args: []string{"-y", "tools"}, Targets: []string{"codex"}}
	wrapper := Entry{Transport: "stdio", Command: "xcli", Args: []string{"--config", source, "mcp", "serve", "tools"}}
	candidate := NativeCandidate{
		Name: "tools", Target: "codex", Fingerprint: wrapper,
		Wrapper: &WrapperReference{SourceConfig: source, Server: "tools"},
	}
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{"tools": server}
	plan, err := BuildImportPlan(cfg, map[string]NativeSnapshot{"codex": {Entries: map[string]NativeCandidate{"tools": candidate}}}, newState(""), ImportBuildOptions{
		SourceConfig: source, Scope: ScopeUser, Targets: []string{"codex"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable || len(plan.Changes) != 1 || plan.Changes[0].Action != ImportActionClaim || plan.Changes[0].desired.Command != "npx" {
		t.Fatalf("generated wrapper was not safely claimed: %#v", plan)
	}
}

func TestApplyImportPlanRestoresSourceWhenStateSaveFails(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "config.yaml")
	original := []byte("version: 1\n# keep\n")
	if err := os.WriteFile(source, original, 0o640); err != nil {
		t.Fatal(err)
	}
	desired := config.MCPServer{Transport: "http", URL: "https://example.com/mcp", Targets: []string{"codex"}}
	plan := ImportPlan{SourceConfig: source, Scope: ScopeUser, Applicable: true, Changes: []ImportChange{{
		Server: "docs", Targets: []string{"codex"}, Action: ImportActionAdd, Status: StatusPlanned,
		desired: &desired, ownership: map[string]Entry{"codex": {Transport: "http", URL: desired.URL}},
	}}}
	state := newState(directory) // Renaming a state file over this directory must fail.
	if err := ApplyImportPlan(&plan, state); err == nil {
		t.Fatal("expected state save failure")
	}
	restored, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(original) {
		t.Fatalf("source was not restored: %q", restored)
	}
	if _, ok := state.Get("codex", "docs"); ok {
		t.Fatal("failed transaction retained in-memory ownership")
	}
}

package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/config"
)

type fakeManager struct {
	entries map[string]Entry
	fail    bool
}

func (m *fakeManager) Load(context.Context) (map[string]Entry, error) {
	result := map[string]Entry{}
	for name, entry := range m.entries {
		result[name] = entry
	}
	return result, nil
}

func (m *fakeManager) Apply(_ context.Context, change Change) error {
	if m.fail {
		return os.ErrPermission
	}
	if change.Action == ActionRemove {
		delete(m.entries, change.Server)
	} else {
		m.entries[change.Server] = *change.desired
	}
	return nil
}

func TestPlanApplyNoopPruneLifecycle(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "config.yaml")
	launcher := filepath.Join(directory, "xcli")
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{"tools": {Transport: "stdio", Command: "server", EnvVars: []string{"TOKEN"}}}
	manager := &fakeManager{entries: map[string]Entry{}}
	managers := map[string]Manager{"codex": manager}
	state := newState(filepath.Join(directory, "state.json"))

	plan, err := BuildPlan(context.Background(), cfg, managers, state, BuildOptions{SourceConfig: source, Launcher: launcher, Targets: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	assertChange(t, plan, ActionAdd, StatusPlanned)
	if plan.Changes[0].desired == nil || len(plan.Changes[0].desired.EnvVars) != 1 {
		t.Fatalf("desired launcher did not forward env_vars: %#v", plan.Changes[0])
	}
	if err := ApplyPlan(context.Background(), &plan, managers, state); err != nil {
		t.Fatal(err)
	}
	if !plan.Applied || plan.Changes[0].Status != StatusApplied {
		t.Fatalf("plan was not applied: %#v", plan)
	}

	plan, err = BuildPlan(context.Background(), cfg, managers, state, BuildOptions{SourceConfig: source, Launcher: launcher, Targets: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	assertChange(t, plan, ActionNoop, StatusSkipped)

	cfg.MCP.Servers = nil
	plan, err = BuildPlan(context.Background(), cfg, managers, state, BuildOptions{SourceConfig: source, Launcher: launcher, Targets: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	assertChange(t, plan, ActionRemove, StatusPlanned)
	if err := ApplyPlan(context.Background(), &plan, managers, state); err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Get("codex", "tools"); ok || len(manager.entries) != 0 {
		t.Fatalf("managed entry was not pruned: %#v %#v", state, manager.entries)
	}
	info, err := os.Stat(state.path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o, want 600", info.Mode().Perm())
	}
}

func TestPlanConflictsAndForce(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "config.yaml")
	desired := Entry{Transport: "http", URL: "https://example.com/mcp"}
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{"docs": {Transport: "http", URL: desired.URL}}
	manager := &fakeManager{entries: map[string]Entry{"docs": {Transport: "http", URL: "https://native.example/mcp"}}}
	managers := map[string]Manager{"codex": manager}
	state := newState(filepath.Join(directory, "state.json"))

	plan, err := BuildPlan(context.Background(), cfg, managers, state, BuildOptions{SourceConfig: source, Targets: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	assertChange(t, plan, ActionConflict, StatusSkipped)
	if plan.Applicable || !strings.Contains(plan.Changes[0].Detail, "not managed") {
		t.Fatalf("expected unowned conflict: %#v", plan)
	}

	plan, err = BuildPlan(context.Background(), cfg, managers, state, BuildOptions{SourceConfig: source, Targets: []string{"codex"}, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	assertChange(t, plan, ActionUpdate, StatusPlanned)
	if !plan.Applicable || !strings.Contains(plan.Changes[0].Detail, "forced") {
		t.Fatalf("force did not resolve conflict: %#v", plan)
	}

	state.Set("codex", "docs", Ownership{SourceConfig: source, Fingerprint: desired.Fingerprint()})
	plan, err = BuildPlan(context.Background(), cfg, managers, state, BuildOptions{SourceConfig: source, Targets: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	assertChange(t, plan, ActionConflict, StatusSkipped)
	if !strings.Contains(plan.Changes[0].Detail, "changed outside") {
		t.Fatalf("expected drift conflict: %#v", plan)
	}

	state.Set("codex", "docs", Ownership{SourceConfig: filepath.Join(directory, "other.yaml"), Fingerprint: manager.entries["docs"].Fingerprint()})
	plan, err = BuildPlan(context.Background(), cfg, managers, state, BuildOptions{SourceConfig: source, Targets: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Changes[0].Detail, "another xcli configuration") {
		t.Fatalf("expected ownership conflict: %#v", plan)
	}
}

func TestPlanTargetsAndUnavailableAreSorted(t *testing.T) {
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{
		"zeta":  {Transport: "http", URL: "https://z.example/mcp", Targets: []string{"codex"}},
		"alpha": {Transport: "http", URL: "https://a.example/mcp"},
	}
	state := newState("")
	manager := &fakeManager{entries: map[string]Entry{}}
	plan, err := BuildPlan(context.Background(), cfg, map[string]Manager{"codex": manager}, state, BuildOptions{
		SourceConfig: "/config", Targets: []string{"codex"}, Unavailable: []string{"claude"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable || len(plan.Changes) != 3 || plan.Changes[0].Target != "claude" || plan.Changes[1].Server != "alpha" || plan.Changes[2].Server != "zeta" {
		t.Fatalf("unexpected sorted plan: %#v", plan)
	}
}

func TestApplyFailureDoesNotClaimOwnership(t *testing.T) {
	directory := t.TempDir()
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{"docs": {Transport: "http", URL: "https://example.com/mcp"}}
	manager := &fakeManager{entries: map[string]Entry{}, fail: true}
	state := newState(filepath.Join(directory, "state.json"))
	plan, err := BuildPlan(context.Background(), cfg, map[string]Manager{"codex": manager}, state, BuildOptions{SourceConfig: "/config", Targets: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyPlan(context.Background(), &plan, map[string]Manager{"codex": manager}, state); err == nil {
		t.Fatal("expected apply failure")
	}
	if _, ok := state.Get("codex", "docs"); ok || plan.Changes[0].Status != StatusFailed {
		t.Fatalf("failed change claimed ownership: %#v %#v", state, plan)
	}
}

func TestProjectPlanUsesPortableLauncherAndIsolatesOwnership(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "config", "xcli.yaml")
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{
		"tools": {Transport: "stdio", Command: "server", EnvVars: []string{"TOKEN"}},
	}
	state := newState(filepath.Join(directory, "state.json"))
	managerA := &fakeManager{entries: map[string]Entry{}}
	optionsA := BuildOptions{
		SourceConfig: source, Launcher: "xcli", Scope: ScopeProject,
		ProjectDir: directory, ProjectConfig: "config/xcli.yaml", Targets: []string{"codex"},
	}
	planA, err := BuildPlan(context.Background(), cfg, map[string]Manager{"codex": managerA}, state, optionsA)
	if err != nil {
		t.Fatal(err)
	}
	assertChange(t, planA, ActionAdd, StatusPlanned)
	desired := planA.Changes[0].desired
	if desired.Command != "xcli" || strings.Join(desired.Args, " ") != "mcp serve --project-config config/xcli.yaml tools" {
		t.Fatalf("unexpected portable entry: %#v", desired)
	}
	if err := ApplyPlan(context.Background(), &planA, map[string]Manager{"codex": managerA}, state); err != nil {
		t.Fatal(err)
	}

	projectB := filepath.Join(directory, "second")
	managerB := &fakeManager{entries: map[string]Entry{}}
	optionsB := optionsA
	optionsB.ProjectDir = projectB
	optionsB.SourceConfig = filepath.Join(projectB, "config", "xcli.yaml")
	planB, err := BuildPlan(context.Background(), cfg, map[string]Manager{"codex": managerB}, state, optionsB)
	if err != nil {
		t.Fatal(err)
	}
	assertChange(t, planB, ActionAdd, StatusPlanned)
	if _, ok := state.GetIn(NamespaceKey(ScopeProject, directory), "codex", "tools"); !ok {
		t.Fatal("first project ownership was not recorded")
	}
	if _, ok := state.GetIn(NamespaceKey(ScopeProject, projectB), "codex", "tools"); ok {
		t.Fatal("ownership leaked into second project")
	}
}

func assertChange(t *testing.T, plan Plan, action, status string) {
	t.Helper()
	if len(plan.Changes) != 1 || plan.Changes[0].Action != action || plan.Changes[0].Status != status {
		t.Fatalf("changes = %#v, want one %s/%s", plan.Changes, action, status)
	}
}

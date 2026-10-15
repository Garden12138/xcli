package mcp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Garden12138/xcli/internal/config"
)

func TestImportCoveragePreservesEnvironmentAndInput(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "config.yaml")
	server := config.MCPServer{Transport: "stdio", Command: "server", Args: []string{"arg"}, Cwd: "/work", Env: map[string]string{"FIXED": "value"}, EnvVars: []string{"TOKEN"}, Targets: []string{"claude"}}
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{"tools": server}
	if err := config.Save(source, cfg); err != nil {
		t.Fatal(err)
	}
	native := cloneMCPServer(server)
	native.Targets = nil
	snapshot := map[string]NativeSnapshot{"codex": {Entries: map[string]NativeCandidate{"tools": {Name: "tools", Target: "codex", Server: native, Fingerprint: Entry{Transport: "stdio", Command: "server", Args: server.Args, Cwd: server.Cwd, EnvVars: server.EnvVars}}}}}
	state := newState(filepath.Join(dir, "state.json"))
	options := ImportBuildOptions{SourceConfig: source, Scope: ScopeUser, Targets: []string{"codex"}}
	plan, err := BuildImportPlan(cfg, snapshot, state, options)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable || plan.Changes[0].Action != ImportActionUpdate {
		t.Fatalf("%+v", plan)
	}
	desired := plan.Changes[0].desired
	if desired.Env["FIXED"] != "value" || desired.Cwd != "/work" || !reflect.DeepEqual(desired.Args, server.Args) || !reflect.DeepEqual(desired.EnvVars, server.EnvVars) {
		t.Fatalf("lost server fields: %+v", desired)
	}
	desired.Env["FIXED"] = "changed"
	desired.Args[0] = "changed"
	desired.EnvVars[0] = "changed"
	desired.Targets[0] = "changed"
	if server.Env["FIXED"] != "value" || server.Args[0] != "arg" || server.EnvVars[0] != "TOKEN" || server.Targets[0] != "claude" || native.Env["FIXED"] != "value" {
		t.Fatal("clone mutated source")
	}
	plan, err = BuildImportPlan(cfg, snapshot, state, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyImportPlan(&plan, state); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := config.Load(source)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.MCP.Servers["tools"].Env["FIXED"] != "value" {
		t.Fatal("apply lost environment")
	}
	repeat, err := BuildImportPlan(loaded, snapshot, state, options)
	if err != nil {
		t.Fatal(err)
	}
	if repeat.Changes[0].Action != ImportActionNoop {
		t.Fatalf("not idempotent: %+v", repeat)
	}
}

func TestNativeUnknownFieldsCannotBeOverwrittenOrPruned(t *testing.T) {
	for _, scope := range []string{ScopeUser, ScopeProject} {
		t.Run(scope, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "native.json")
			original := []byte(`{"mcp":{"docs":{"type":"remote","url":"https://example.com","headers":{"Authorization":"private"},"nested":{"enabled":true}}}}`)
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			manager := &openCodeManager{path: path}
			managers := map[string]Manager{"opencode": manager}
			cfg := config.Defaults()
			cfg.MCP.Servers = map[string]config.MCPServer{"docs": {Transport: "http", URL: "https://new.example.com"}}
			state := newState(filepath.Join(dir, "state.json"))
			namespace := NamespaceKey(scope, dir)
			state.SetIn(namespace, "opencode", "docs", Ownership{SourceConfig: "/source", Fingerprint: Entry{Transport: "http", URL: "https://example.com"}.Fingerprint()})
			opts := BuildOptions{Scope: scope, ProjectDir: dir, ProjectConfig: "xcli.yaml", SourceConfig: "/source", Targets: []string{"opencode"}, Force: true}
			for _, remove := range []bool{false, true} {
				if remove {
					cfg.MCP.Servers = nil
				}
				plan, err := BuildPlan(context.Background(), cfg, managers, state, opts)
				if err != nil {
					t.Fatal(err)
				}
				if plan.Applicable || plan.Changes[0].Action != ActionConflict {
					t.Fatalf("unsafe plan: %+v", plan)
				}
				if err := ApplyPlan(context.Background(), &plan, managers, state); err == nil {
					t.Fatal("unsafe apply succeeded")
				}
				now, _ := os.ReadFile(path)
				if !reflect.DeepEqual(now, original) {
					t.Fatal("native fields changed")
				}
			}
		})
	}
}

func TestAddRechecksNativeNameIncludingUnsupportedEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "native.json")
	manager := &openCodeManager{path: path}
	managers := map[string]Manager{"opencode": manager}
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{"docs": {Transport: "http", URL: "https://example.com"}}
	state := newState(filepath.Join(dir, "state.json"))
	plan, err := BuildPlan(context.Background(), cfg, managers, state, BuildOptions{SourceConfig: "/source", Targets: []string{"opencode"}})
	if err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"mcp":{"docs":{"vendor_specific":true}}}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPlan(context.Background(), &plan, managers, state); err == nil {
		t.Fatal("overwrote entry added after plan")
	}
	now, _ := os.ReadFile(path)
	if !reflect.DeepEqual(now, original) {
		t.Fatal("native entry changed")
	}
}

func TestStateRejectsStaleConcurrentWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first, second := newState(path), newState(path)
	first.Set("codex", "one", Ownership{SourceConfig: "/one"})
	second.Set("claude", "two", Ownership{SourceConfig: "/two"})
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}
	if err := second.Save(); err == nil {
		t.Fatal("stale namespace writer overwrote state")
	}
	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte("one")) || bytes.Contains(data, []byte("two")) {
		t.Fatalf("%s", data)
	}
}

func TestAllNativeFormatsProtectUnknownFields(t *testing.T) {
	for target, document := range map[string]string{
		"claude":   `{"mcpServers":{"docs":{"type":"http","url":"https://example.com","headers":{"Authorization":"private"}}}}`,
		"gemini":   `{"mcpServers":{"docs":{"httpUrl":"https://example.com","headers":{"Authorization":"private"}}}}`,
		"opencode": `{"mcp":{"docs":{"type":"remote","url":"https://example.com","nested":{"unknown":true}}}}`,
		"codex":    "[mcp_servers.docs]\nurl = \"https://example.com\"\n[mcp_servers.docs.http_headers]\nAuthorization = \"private\"\n",
	} {
		t.Run(target, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "native")
			if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := checkNativeName(target, path, "docs"); err == nil {
				t.Fatal("advanced native entry was considered absent")
			}
			now, _ := os.ReadFile(path)
			if string(now) != document {
				t.Fatal("native file changed")
			}
		})
	}
}

type stateFailureManager struct {
	fakeManager
	path string
}

func (m *stateFailureManager) Apply(ctx context.Context, change Change) error {
	if err := m.fakeManager.Apply(ctx, change); err != nil {
		return err
	}
	if err := os.Rename(m.path, m.path+".saved"); err != nil {
		return err
	}
	return os.Mkdir(m.path, 0o700)
}

func TestNativeWriteThenStateFailureIsVisible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	manager := &stateFailureManager{fakeManager: fakeManager{entries: map[string]Entry{}}, path: path}
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{"docs": {Transport: "http", URL: "https://example.com"}}
	managers := map[string]Manager{"codex": manager}
	state := newState(path)
	opts := BuildOptions{SourceConfig: "/source", Targets: []string{"codex"}}
	plan, err := BuildPlan(context.Background(), cfg, managers, state, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyPlan(context.Background(), &plan, managers, state); err == nil || plan.Applied || plan.Changes[0].Status != StatusFailed {
		t.Fatalf("%+v %v", plan, err)
	}
	if _, exists := manager.entries["docs"]; !exists {
		t.Fatal("did not inject a post-native-write failure")
	}
	repeat, err := BuildPlan(context.Background(), cfg, managers, newState(path), opts)
	if err != nil {
		t.Fatal(err)
	}
	if repeat.Applicable || repeat.Changes[0].Action != ActionConflict {
		t.Fatalf("unconfirmed write became success: %+v", repeat)
	}
}

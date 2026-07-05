package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadProjectNativeSnapshotsNormalizeCommonFields(t *testing.T) {
	project := t.TempDir()
	source := filepath.Join(project, ".xcli", "config.yaml")
	options := ImportReadOptions{Scope: ScopeProject, ProjectDir: project, SourceConfig: source}
	fixtures := map[string]struct {
		path string
		data string
	}{
		"claude": {
			path: filepath.Join(project, ".mcp.json"),
			data: `{"mcpServers":{"tools":{"type":"stdio","command":"npx","args":["-y","tools"],"cwd":"tools","env":{"TOKEN":"${TOKEN}"}},"secret":{"type":"stdio","command":"server","env":{"TOKEN":"do-not-copy"}}}}`,
		},
		"gemini": {
			path: filepath.Join(project, ".gemini", "settings.json"),
			data: `{"mcpServers":{"docs":{"httpUrl":"https://example.com/mcp"},"headers":{"httpUrl":"https://example.com/mcp","headers":{"Authorization":"do-not-copy"}}}}`,
		},
		"opencode": {
			path: filepath.Join(project, "opencode.json"),
			data: `{"mcp":{"tools":{"type":"local","command":["npx","-y","tools"],"cwd":"tools","environment":{"TOKEN":"{env:TOKEN}"}},"off":{"type":"remote","url":"https://example.com/mcp","enabled":false}}}`,
		},
		"codex": {
			path: filepath.Join(project, ".codex", "config.toml"),
			data: `[mcp_servers.tools]
command = "npx"
args = ["-y", "tools"]
cwd = "tools"
env_vars = ["TOKEN"]

[mcp_servers.remote-env]
command = "server"
env_vars = [{ name = "TOKEN", source = "remote" }]
`,
		},
	}
	for target, fixture := range fixtures {
		t.Run(target, func(t *testing.T) {
			if err := os.MkdirAll(filepath.Dir(fixture.path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fixture.path, []byte(fixture.data), 0o600); err != nil {
				t.Fatal(err)
			}
			snapshot, err := ReadNativeSnapshot(target, fixture.path, options)
			if err != nil {
				t.Fatal(err)
			}
			if !snapshot.Exists {
				t.Fatal("native configuration was not detected")
			}
			switch target {
			case "claude", "opencode", "codex":
				candidate := snapshot.Entries["tools"]
				if candidate.Unsupported != "" || candidate.Server.Command != "npx" || candidate.Server.Cwd != "../tools" || len(candidate.Server.EnvVars) != 1 {
					t.Fatalf("unexpected normalized candidate: %#v", candidate)
				}
			case "gemini":
				candidate := snapshot.Entries["docs"]
				if candidate.Unsupported != "" || candidate.Server.URL != "https://example.com/mcp" {
					t.Fatalf("unexpected HTTP candidate: %#v", candidate)
				}
			}
		})
	}
}

func TestNativeInspectionRejectsLossyValuesWithoutLeakingThem(t *testing.T) {
	options := ImportReadOptions{Scope: ScopeUser, SourceConfig: "/config.yaml"}
	tests := []struct {
		target string
		data   string
		name   string
	}{
		{target: "claude", data: `{"mcpServers":{"bad":{"type":"stdio","command":"server","env":{"TOKEN":"super-secret"}}}}`, name: "bad"},
		{target: "gemini", data: `{"mcpServers":{"bad":{"httpUrl":"https://example.com/mcp","headers":{"X-Key":"super-secret"}}}}`, name: "bad"},
		{target: "opencode", data: `{"mcp":{"bad_name":{"type":"remote","url":"https://example.com/mcp"}}}`, name: "bad_name"},
	}
	for _, test := range tests {
		t.Run(test.target, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(test.data), 0o600); err != nil {
				t.Fatal(err)
			}
			snapshot, err := ReadNativeSnapshot(test.target, path, options)
			if err != nil {
				t.Fatal(err)
			}
			reason := snapshot.Entries[test.name].Unsupported
			if reason == "" || strings.Contains(reason, "super-secret") {
				t.Fatalf("unsafe unsupported reason %q", reason)
			}
		})
	}
}

func TestRecognizeGeneratedMCPWrappers(t *testing.T) {
	user := recognizeWrapper(Entry{Args: []string{"--config", "/tmp/config.yaml", "mcp", "serve", "tools"}}, ImportReadOptions{Scope: ScopeUser})
	if user == nil || user.Server != "tools" || user.SourceConfig != "/tmp/config.yaml" {
		t.Fatalf("unexpected user wrapper: %#v", user)
	}
	project := t.TempDir()
	projectWrapper := recognizeWrapper(Entry{Args: []string{"mcp", "serve", "--project-config", ".xcli/config.yaml", "tools"}}, ImportReadOptions{Scope: ScopeProject, ProjectDir: project})
	if projectWrapper == nil || projectWrapper.Server != "tools" || projectWrapper.SourceConfig != filepath.Join(project, ".xcli", "config.yaml") {
		t.Fatalf("unexpected project wrapper: %#v", projectWrapper)
	}
}

func TestNativeInspectionRejectsNonPortableCwdAndExpansion(t *testing.T) {
	user := ImportReadOptions{Scope: ScopeUser, SourceConfig: "/config.yaml"}
	if candidate := inspectGeminiNative([]byte(`{"command":"server","cwd":"relative"}`), user); candidate.Unsupported == "" || !strings.Contains(candidate.Unsupported, "relative cwd") {
		t.Fatalf("user-relative cwd was accepted: %#v", candidate)
	}
	project := ImportReadOptions{Scope: ScopeProject, ProjectDir: "/project", SourceConfig: "/project/.xcli/config.yaml"}
	if candidate := inspectOpenCodeNative([]byte(`{"type":"local","command":["server"],"cwd":"/absolute"}`), project); candidate.Unsupported == "" || !strings.Contains(candidate.Unsupported, "absolute cwd") {
		t.Fatalf("project-absolute cwd was accepted: %#v", candidate)
	}
	if candidate := inspectClaudeNative([]byte(`{"type":"stdio","command":"${SERVER_BIN}","args":[]}`), project); candidate.Unsupported == "" || !strings.Contains(candidate.Unsupported, "variable expansion") {
		t.Fatalf("expanded command was accepted: %#v", candidate)
	}
	data := []byte("[mcp_servers.tools]\ncommand = \"server\"\nstartup_timeout_sec = 10\n")
	entries, err := readCodexNativeEntries(data, project)
	if err != nil {
		t.Fatal(err)
	}
	if entries["tools"].Unsupported == "" {
		t.Fatalf("advanced Codex field was accepted: %#v", entries["tools"])
	}
}

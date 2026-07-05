package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/config"
)

func TestVendorEntryParsers(t *testing.T) {
	claude, ok := parseClaudeEntry(json.RawMessage(`{"type":"stdio","command":"xcli","args":["mcp"],"env":{"TOKEN":"${TOKEN}"}}`))
	if !ok || claude.Command != "xcli" || len(claude.Args) != 1 || len(claude.EnvVars) != 1 {
		t.Fatalf("unexpected Claude entry: %#v", claude)
	}
	gemini, ok := parseGeminiEntry(json.RawMessage(`{"httpUrl":"https://example.com/mcp"}`))
	if !ok || gemini.Transport != "http" || gemini.URL == "" {
		t.Fatalf("unexpected Gemini entry: %#v", gemini)
	}
	opencode, ok := parseOpenCodeEntry(json.RawMessage(`{"type":"local","command":["xcli","mcp","serve","tools"],"environment":{"TOKEN":"{env:TOKEN}"}}`))
	if !ok || opencode.Command != "xcli" || len(opencode.Args) != 3 || len(opencode.EnvVars) != 1 {
		t.Fatalf("unexpected OpenCode entry: %#v", opencode)
	}
	codexData := []byte(`[{"name":"docs","transport":{"type":"streamable_http","url":"https://example.com/mcp"}}]`)
	codex, err := parseCodexEntries(codexData)
	if err != nil || codex["docs"].Transport != "http" {
		t.Fatalf("unexpected Codex entries: %#v, %v", codex, err)
	}
}

func TestOpenCodeManagerPreservesCommentsAndBacksUp(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "opencode.json")
	original := `{
  // keep this comment
  "theme": "dark",
  "mcp": {
    "native": {"type": "remote", "url": "https://native.example/mcp"},
  },
}
`
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	manager := &openCodeManager{path: path}
	desired := Entry{Transport: "stdio", Command: "/usr/bin/xcli", Args: []string{"mcp", "serve", "tools"}, EnvVars: []string{"TOKEN"}}
	change := Change{Target: "opencode", Server: "tools", Action: ActionAdd, desired: &desired}
	if err := manager.Apply(context.Background(), change); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "keep this comment") || !strings.Contains(string(data), "native") {
		t.Fatalf("OpenCode config lost comments or unrelated entries: %s", data)
	}
	entries, err := manager.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if entries["tools"].Command != "/usr/bin/xcli" || len(entries["tools"].EnvVars) != 1 || entries["native"].URL == "" {
		t.Fatalf("unexpected OpenCode entries: %#v", entries)
	}
	backup, err := os.ReadFile(path + ".xcli.bak")
	if err != nil || string(backup) != original {
		t.Fatalf("unexpected backup: %q, %v", backup, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("OpenCode config mode = %v, %v", info.Mode().Perm(), err)
	}

	current := entries["tools"]
	remove := Change{Target: "opencode", Server: "tools", Action: ActionRemove, current: &current}
	if err := manager.Apply(context.Background(), remove); err != nil {
		t.Fatal(err)
	}
	entries, err = manager.Load(context.Background())
	if err != nil || entries["tools"].Command != "" || entries["native"].URL == "" {
		t.Fatalf("OpenCode remove failed: %#v, %v", entries, err)
	}
}

func TestCommandManagersUseNativeMCPCommands(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "fake-cli")
	scriptData := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$MCP_COMMAND_LOG\"\n"
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		target string
		entry  Entry
		want   string
	}{
		{target: "claude", entry: Entry{Transport: "stdio", Command: "/bin/xcli", Args: []string{"mcp", "serve", "tools"}, EnvVars: []string{"TOKEN"}}, want: "mcp add-json --scope user tools {\"args\":[\"mcp\",\"serve\",\"tools\"],\"command\":\"/bin/xcli\",\"env\":{\"TOKEN\":\"${TOKEN}\"},\"type\":\"stdio\"}"},
		{target: "codex", entry: Entry{Transport: "http", URL: "https://example.com/mcp"}, want: "mcp add docs --url https://example.com/mcp"},
		{target: "gemini", entry: Entry{Transport: "stdio", Command: "/bin/xcli", Args: []string{"mcp", "serve", "tools"}, EnvVars: []string{"TOKEN"}}, want: "mcp add --scope user --env TOKEN=$TOKEN docs /bin/xcli mcp serve tools"},
	}
	for _, test := range tests {
		t.Run(test.target, func(t *testing.T) {
			logPath := filepath.Join(directory, test.target+".log")
			manager := &commandManager{
				target:      test.target,
				config:      config.AgentConfig{Command: script},
				environment: append(os.Environ(), "MCP_COMMAND_LOG="+logPath),
			}
			change := Change{Target: test.target, Server: "docs", Action: ActionAdd, desired: &test.entry}
			if test.target == "claude" {
				change.Server = "tools"
			}
			if err := manager.Apply(context.Background(), change); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(data)) != test.want {
				t.Fatalf("command = %q, want %q", strings.TrimSpace(string(data)), test.want)
			}
		})
	}
}

func TestCommandManagerRestoresEntryWhenUpdateFails(t *testing.T) {
	directory := t.TempDir()
	logPath := filepath.Join(directory, "commands.log")
	script := filepath.Join(directory, "fake-codex")
	scriptData := `#!/bin/sh
printf '%s\n' "$*" >> "$MCP_COMMAND_LOG"
case "$*" in
  *new.example*) echo 'add failed' >&2; exit 9 ;;
esac
`
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	manager := &commandManager{
		target:      "codex",
		config:      config.AgentConfig{Command: script},
		environment: append(os.Environ(), "MCP_COMMAND_LOG="+logPath),
	}
	current := Entry{Transport: "http", URL: "https://old.example/mcp"}
	desired := Entry{Transport: "http", URL: "https://new.example/mcp"}
	change := Change{Target: "codex", Server: "docs", Action: ActionUpdate, current: &current, desired: &desired}
	if err := manager.Apply(context.Background(), change); err == nil || !strings.Contains(err.Error(), "add failed") {
		t.Fatalf("expected update failure, got %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	for _, command := range []string{
		"mcp remove docs", "mcp add docs --url https://new.example/mcp", "mcp add docs --url https://old.example/mcp",
	} {
		if !strings.Contains(log, command) {
			t.Fatalf("rollback log %q missing %q", log, command)
		}
	}
}

func TestSetCodexEnvVarsPreservesOtherConfiguration(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.toml")
	original := "model = \"gpt\"\n\n[mcp_servers.\"local.tools\"]\ncommand = \"xcli\"\nargs = [\"mcp\"]\n\n[other]\nenabled = true\n"
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := &commandManager{environment: []string{"CODEX_HOME=" + directory}}
	if err := manager.setCodexEnvVars("local.tools", []string{"TOKEN_B", "TOKEN_A"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `env_vars = ["TOKEN_A","TOKEN_B"]`) || !strings.Contains(text, "[other]\nenabled = true") || !strings.Contains(text, `model = "gpt"`) {
		t.Fatalf("Codex config was not patched safely: %s", text)
	}
}

func TestProjectJSONManagersPreserveCommentsAndUnrelatedConfiguration(t *testing.T) {
	directory := t.TempDir()
	managers, err := NewProjectManagers(config.Defaults(), directory)
	if err != nil {
		t.Fatal(err)
	}
	fixtures := map[string]struct {
		path string
		data string
	}{
		"claude": {
			path: filepath.Join(directory, ".mcp.json"),
			data: "{\n  // claude comment\n  \"keep\": true,\n  \"mcpServers\": {\"native\": {\"type\": \"http\", \"url\": \"https://native.example/mcp\"}}\n}\n",
		},
		"gemini": {
			path: filepath.Join(directory, ".gemini", "settings.json"),
			data: "{\n  // gemini comment\n  \"keep\": true,\n  \"mcpServers\": {\"native\": {\"httpUrl\": \"https://native.example/mcp\"}}\n}\n",
		},
		"opencode": {
			path: filepath.Join(directory, "opencode.json"),
			data: "{\n  // opencode comment\n  \"keep\": true,\n  \"mcp\": {\"native\": {\"type\": \"remote\", \"url\": \"https://native.example/mcp\"}}\n}\n",
		},
	}
	desired := Entry{
		Transport: "stdio", Command: "xcli",
		Args: []string{"mcp", "serve", "--project-config", ".xcli/config.yaml", "tools"}, EnvVars: []string{"TOKEN"},
	}
	for target, fixture := range fixtures {
		t.Run(target, func(t *testing.T) {
			if err := os.MkdirAll(filepath.Dir(fixture.path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fixture.path, []byte(fixture.data), 0o640); err != nil {
				t.Fatal(err)
			}
			change := Change{Target: target, Server: "tools", Action: ActionAdd, desired: &desired}
			if err := managers[target].Apply(context.Background(), change); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), target+" comment") || !strings.Contains(string(data), `"keep"`) || !strings.Contains(string(data), `"native"`) {
				t.Fatalf("project config lost comments or unrelated data: %s", data)
			}
			entries, err := managers[target].Load(context.Background())
			if err != nil || entries["tools"].Command != "xcli" || len(entries["tools"].EnvVars) != 1 || entries["native"].URL == "" {
				t.Fatalf("unexpected project entries: %#v, %v", entries, err)
			}
			info, err := os.Stat(fixture.path)
			if err != nil || info.Mode().Perm() != 0o640 {
				t.Fatalf("project config mode = %v, %v", info.Mode().Perm(), err)
			}
			if _, err := os.Stat(fixture.path + ".xcli.bak"); !os.IsNotExist(err) {
				t.Fatalf("project sync left a backup file: %v", err)
			}
			current := entries["tools"]
			if err := managers[target].Apply(context.Background(), Change{Target: target, Server: "tools", Action: ActionRemove, current: &current}); err != nil {
				t.Fatal(err)
			}
			entries, err = managers[target].Load(context.Background())
			if err != nil || entries["tools"].Command != "" || entries["native"].URL == "" {
				t.Fatalf("project removal damaged unrelated entries: %#v, %v", entries, err)
			}
		})
	}
}

func TestProjectCodexManagerPreservesTOMLCommentsAndPermissions(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "# keep comment\nmodel = \"gpt-5\"\n\n[mcp_servers.\"native\"]\nurl = \"https://native.example/mcp\"\n\n[other]\nenabled = true\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	manager := &projectCodexManager{path: path}
	desired := Entry{Transport: "stdio", Command: "xcli", Args: []string{"mcp", "serve", "--project-config", "config.yaml", "tools"}, EnvVars: []string{"Z", "A"}}
	if err := manager.Apply(context.Background(), Change{Target: "codex", Server: "tools", Action: ActionAdd, desired: &desired}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, value := range []string{"# keep comment", `model = "gpt-5"`, `[other]`, `[mcp_servers."tools"]`, `env_vars = ["A","Z"]`} {
		if !strings.Contains(text, value) {
			t.Fatalf("Codex project config %q missing %q", text, value)
		}
	}
	entries, err := manager.Load(context.Background())
	if err != nil || entries["tools"].Command != "xcli" || entries["native"].URL == "" {
		t.Fatalf("unexpected Codex project entries: %#v, %v", entries, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("Codex project mode = %v, %v", info.Mode().Perm(), err)
	}
	current := entries["tools"]
	if err := manager.Apply(context.Background(), Change{Target: "codex", Server: "tools", Action: ActionRemove, current: &current}); err != nil {
		t.Fatal(err)
	}
	entries, err = manager.Load(context.Background())
	if err != nil || entries["tools"].Command != "" || entries["native"].URL == "" {
		t.Fatalf("Codex project removal damaged unrelated entries: %#v, %v", entries, err)
	}
}

func TestProjectCodexUpdateReplacesWholeServerTable(t *testing.T) {
	current := Entry{Transport: "http", URL: "https://old.example/mcp"}
	desired := Entry{Transport: "http", URL: "https://new.example/mcp"}
	data := []byte("[mcp_servers.\"tools\"]\nurl = \"https://old.example/mcp\"\n\n[mcp_servers.\"tools\".headers]\nAuthorization = \"old\"\n\n# keep other comment\n[other]\nkeep = true\n")
	updated, err := updateCodexProjectDocument(data, Change{
		Server: "tools", Action: ActionUpdate, current: &current, desired: &desired,
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	if strings.Contains(text, "headers") || strings.Contains(text, "Authorization") || !strings.Contains(text, "https://new.example/mcp") || !strings.Contains(text, "# keep other comment\n[other]\nkeep = true") {
		t.Fatalf("Codex full table replacement was not targeted: %s", text)
	}
}

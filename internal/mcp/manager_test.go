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

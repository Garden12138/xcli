package mcp

import (
	"bytes"
	"context"
	"fmt"
	"os"
)

func unchangedFile(path string, original []byte, existed bool) func() error {
	return func() error {
		current, err := os.ReadFile(path)
		if !existed && os.IsNotExist(err) {
			return nil
		}
		if err != nil || !existed || !bytes.Equal(current, original) {
			return fmt.Errorf("native configuration changed before replacement; rerun the plan")
		}
		return nil
	}
}

func checkNativeName(target, path, name string) error {
	snapshot, err := ReadNativeSnapshot(target, path, ImportReadOptions{})
	if err != nil {
		return fmt.Errorf("cannot prove native MCP name is absent: %w", err)
	}
	if _, exists := snapshot.Entries[name]; exists {
		return fmt.Errorf("native MCP entry %q exists; replacement is retired", name)
	}
	return nil
}

func (m *commandManager) CheckAdd(_ context.Context, name string) error {
	if len(m.config.Args) > 0 {
		return fmt.Errorf("cannot prove authoritative MCP config with custom native arguments")
	}
	environment := environmentMap(m.environment)
	for _, key := range []string{"CLAUDE_CONFIG_DIR", "GEMINI_CLI_HOME"} {
		if environment[key] != "" || os.Getenv(key) != "" {
			return fmt.Errorf("cannot prove authoritative MCP config with %s", key)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if configured := environment["HOME"]; configured != "" && configured != home {
		return fmt.Errorf("cannot prove authoritative native MCP config with a different HOME")
	}
	if m.configPath == "" {
		return fmt.Errorf("authoritative native configuration is unavailable")
	}
	return checkNativeName(m.target, m.configPath, name)
}
func (m *openCodeManager) CheckAdd(_ context.Context, name string) error {
	if os.Getenv("OPENCODE_CONFIG") != "" || os.Getenv("OPENCODE_CONFIG_CONTENT") != "" {
		return fmt.Errorf("cannot prove authoritative OpenCode config with overrides")
	}
	return checkNativeName("opencode", m.path, name)
}
func (m *projectJSONManager) CheckAdd(_ context.Context, name string) error {
	return checkNativeName(m.target, m.path, name)
}
func (m *projectCodexManager) CheckAdd(_ context.Context, name string) error {
	return checkNativeName("codex", m.path, name)
}

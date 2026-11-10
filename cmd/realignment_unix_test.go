//go:build darwin || linux

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR4DoctorNeverUsesLegacyLoginAndPreservesConfiguration(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-native")
	calls := filepath.Join(dir, "calls")
	data := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + calls + "'\ncase \"$1\" in\n--version) echo fixture-1;;\n--help) echo 'exec resume --print --output-format --conversation';;\n*) exit 97;;\nesac\n"
	if err := os.WriteFile(script, []byte(data), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "legacy.yaml")
	config := "version: 1\ndefault_agent: codex\nrecording:\n  output: false\nrouting:\n  rules:\n    - name: old-rule\n      prompt_regex: .\n      agent: cursor\nagents:\n"
	for _, name := range []string{"codex", "cursor", "antigravity"} {
		config += "  " + name + ":\n    command: " + script + "\n    auth_args: [login, never-run]\n"
	}
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"codex", "cursor", "antigravity"} {
		c := newRootCommand()
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetErr(&out)
		c.SetArgs([]string{"--config", path, "doctor", name, "--json"})
		if err := c.Execute(); err != nil {
			t.Fatal(name, err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != config {
		t.Fatal("legacy config changed")
	}
	observed, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(observed)), "\n") {
		if line != "--version" && line != "--help" {
			t.Fatal("doctor invoked authentication", line)
		}
	}
	if len(strings.Split(strings.TrimSpace(string(observed)), "\n")) != 6 {
		t.Fatal("unexpected native probes")
	}
}

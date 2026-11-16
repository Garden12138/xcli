package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Garden12138/xcli/internal/config"
)

func TestConfigInitPreservesExisting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XCLI_HOME", home)
	run := func(args ...string) error {
		root := newRootCommand()
		root.SetOut(new(bytes.Buffer))
		root.SetArgs(args)
		return root.Execute()
	}
	if err := run("config", "init"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "config.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := run("config", "init"); err == nil {
		t.Fatal("existing config was overwritten")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("existing configuration changed")
	}
	if _, err := config.Parse(after); err != nil {
		t.Fatal(err)
	}
}

func TestAgentsUsesConfiguredFakePath(t *testing.T) {
	home, bin := t.TempDir(), t.TempDir()
	t.Setenv("XCLI_HOME", home)
	for _, name := range []string{"codex", "cursor-agent", "agy"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nif [ \"$1\" = --version ]; then printf 'fake 1.0\\n'; else printf 'ok\\n'; fi\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	pathJSON, _ := json.Marshal(bin + ":/bin:/usr/bin")
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), append([]byte("path: "), pathJSON...), 0600); err != nil {
		t.Fatal(err)
	}
	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"agents", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Name, Version        string
		Installed, Available bool
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil || len(rows) != 3 {
		t.Fatalf("agents: %s, %v", &out, err)
	}
	for _, row := range rows {
		if !row.Installed || !row.Available || row.Version != "fake 1.0" {
			t.Fatalf("agent: %+v", row)
		}
	}
}

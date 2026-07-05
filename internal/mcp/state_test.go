package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStateMigratesVersionOneOwnershipToUserScope(t *testing.T) {
	dataRoot := t.TempDir()
	previous, hadPrevious := os.LookupEnv("XDG_DATA_HOME")
	if err := os.Setenv("XDG_DATA_HOME", dataRoot); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if hadPrevious {
			_ = os.Setenv("XDG_DATA_HOME", previous)
		} else {
			_ = os.Unsetenv("XDG_DATA_HOME")
		}
	}()
	path := filepath.Join(dataRoot, "xcli", "mcp-sync.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]interface{}{
		"version": 1,
		"entries": map[string]interface{}{
			"codex": map[string]interface{}{
				"tools": map[string]string{"source_config": "/config", "fingerprint": "abc"},
			},
		},
	}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}
	owner, ok := state.Get("codex", "tools")
	if !ok || owner.SourceConfig != "/config" || state.Version != stateVersion {
		t.Fatalf("legacy state was not migrated: %#v", state)
	}
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) == string(data) || !json.Valid(written) {
		t.Fatalf("state was not rewritten as version 2: %s", written)
	}
}

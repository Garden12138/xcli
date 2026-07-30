package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/runstore"
)

func TestRunsListFiltersAndWritesJSONL(t *testing.T) {
	setCommandEnvironment(t, "XDG_DATA_HOME", t.TempDir())
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	project := t.TempDir()
	records := []runstore.Record{
		{ID: "run-match", Kind: "run", Agent: "codex", Status: "success", Cwd: project, StartedAt: now},
		{ID: "workflow-match", Kind: "workflow", Status: "failed", Cwd: project, StartedAt: now, Steps: []runstore.StepRecord{{Agent: "claude"}}},
		{ID: "run-old", Kind: "run", Agent: "codex", Status: "success", Cwd: filepath.Join(project, "other"), StartedAt: now.Add(-48 * time.Hour)},
	}
	for _, record := range records {
		if err := store.Save(record); err != nil {
			t.Fatal(err)
		}
	}

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"runs", "list", "--agent", "codex", "--kind", "run", "--status", "success", "--cwd", project, "--since", "24h", "--jsonl"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(stdout.String()))
	var record runstore.Record
	if err := decoder.Decode(&record); err != nil {
		t.Fatalf("decode JSONL %q: %v", stdout.String(), err)
	}
	if record.ID != "run-match" {
		t.Fatalf("unexpected filtered record: %#v output=%q", record, stdout.String())
	}
	var extra runstore.Record
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("expected a single JSONL record, got %v", err)
	}
}

func TestRunsListRejectsConflictingJSONFormatsAndInvalidSince(t *testing.T) {
	setCommandEnvironment(t, "XDG_DATA_HOME", t.TempDir())
	for _, args := range [][]string{
		{"runs", "list", "--json", "--jsonl"},
		{"runs", "list", "--since", "not-a-time"},
	} {
		root := newRootCommand()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Fatalf("%v unexpectedly succeeded", args)
		}
	}
}

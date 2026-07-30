package runstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
)

func TestSaveLoadAndList(t *testing.T) {
	store := &Store{root: filepath.Join(t.TempDir(), "runs")}
	cost := 0.0
	record := Record{
		ID: "run-test", Kind: "run", Agent: "codex", StartedAt: time.Now().UTC(), Status: "success",
		Usage: &agent.Usage{InputTokens: 10, TotalTokens: 10, EstimatedCostUSD: &cost},
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Agent != "codex" || loaded.Usage == nil || loaded.Usage.InputTokens != 10 ||
		loaded.Usage.EstimatedCostUSD == nil || *loaded.Usage.EstimatedCostUSD != 0 {
		t.Fatalf("unexpected record: %#v", loaded)
	}
	info, err := os.Stat(filepath.Join(store.root, "run-test.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("record mode = %o, want 600", info.Mode().Perm())
	}
	records, err := store.List()
	if err != nil || len(records) != 1 {
		t.Fatalf("list = %#v, %v", records, err)
	}
}

func TestLoadLegacyRecordWithoutRoutingMetadata(t *testing.T) {
	directory := t.TempDir()
	store := &Store{root: directory}
	data := []byte(`{"id":"run-legacy","kind":"run","agent":"codex","cwd":"/tmp","status":"success","exit_code":0}`)
	if err := os.WriteFile(filepath.Join(directory, "run-legacy.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load("run-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if record.Agent != "codex" || record.SelectionSource != "" || record.RouteRule != "" || record.Usage != nil {
		t.Fatalf("unexpected legacy record: %#v", record)
	}
}

func TestDeleteRemovesOnlyValidatedRecord(t *testing.T) {
	store := &Store{root: t.TempDir()}
	record := Record{ID: "run-delete", Kind: "run", Status: "success"}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(record.ID); !os.IsNotExist(err) {
		t.Fatalf("deleted record load error = %v", err)
	}
	for _, id := range []string{"", "../outside", `..\\outside`} {
		if err := store.Delete(id); err == nil {
			t.Fatalf("Delete(%q) succeeded", id)
		}
	}
}

func TestFilterAndProjectMatching(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	project := filepath.Join(t.TempDir(), "project")
	other := filepath.Join(t.TempDir(), "other")
	records := []Record{
		{ID: "run-one", Kind: "run", Agent: "codex", Status: "success", Cwd: project, StartedAt: now},
		{ID: "workflow-one", Kind: "workflow", Status: "failed", Cwd: filepath.Join(project, "sub"), StartedAt: now.Add(-time.Hour), Steps: []StepRecord{{Agent: "claude"}}},
		{ID: "run-old", Kind: "run", Agent: "codex", Status: "success", Cwd: other, StartedAt: now.Add(-48 * time.Hour)},
	}
	since := now.Add(-2 * time.Hour)
	filtered := Filter(records, FilterOptions{Agent: "claude", Kind: "workflow", Status: "failed", Since: &since})
	if len(filtered) != 1 || filtered[0].ID != "workflow-one" {
		t.Fatalf("unexpected filtered records: %#v", filtered)
	}
	if !WithinProject(records[1], project) || WithinProject(records[2], project) {
		t.Fatal("project containment did not match expected records")
	}
	isolated := Record{Repository: project, Cwd: filepath.Join(other, "managed-worktree")}
	if !WithinProject(isolated, project) {
		t.Fatal("isolated worktree repository was not associated with its project")
	}
}

func TestNewDataPathAndOutputStorage(t *testing.T) {
	dataHome := t.TempDir()
	oldDataHome, hadDataHome := os.LookupEnv("XDG_DATA_HOME")
	if err := os.Setenv("XDG_DATA_HOME", dataHome); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadDataHome {
			_ = os.Setenv("XDG_DATA_HOME", oldDataHome)
		} else {
			_ = os.Unsetenv("XDG_DATA_HOME")
		}
	})
	store, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if store.Root() != filepath.Join(dataHome, "xcli", "runs") {
		t.Fatalf("store root = %q", store.Root())
	}
	path, err := store.SaveOutput("run-output", "stdout.log", []byte("captured"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "captured" || info.Mode().Perm() != 0o600 {
		t.Fatalf("output = %q mode=%o", data, info.Mode().Perm())
	}
	for _, target := range [][2]string{
		{"", "stdout.log"},
		{"../outside", "stdout.log"},
		{"run-output", "../outside"},
		{"run-output", ""},
	} {
		if _, err := store.SaveOutput(target[0], target[1], nil); err == nil {
			t.Fatalf("SaveOutput(%q, %q) succeeded", target[0], target[1])
		}
	}
}

func TestListSortsRecordsAndSkipsCorruptFiles(t *testing.T) {
	store := &Store{root: filepath.Join(t.TempDir(), "runs")}
	older := Record{ID: "run-older", StartedAt: time.Now().UTC().Add(-time.Hour)}
	newer := Record{ID: "run-newer", StartedAt: time.Now().UTC()}
	if err := store.Save(older); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(newer); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.root, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.root, "ignored.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ID != newer.ID || records[1].ID != older.ID {
		t.Fatalf("unexpected records: %#v", records)
	}
	if _, err := store.Load("broken"); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected corrupt-record error, got %v", err)
	}
	for _, id := range []string{"", "../outside", `..\\outside`} {
		if err := store.Save(Record{ID: id}); err == nil {
			t.Fatalf("Save(%q) succeeded", id)
		}
		if _, err := store.Load(id); err == nil {
			t.Fatalf("Load(%q) succeeded", id)
		}
	}
}

func TestNewIDIncludesPrefixAndIsUnique(t *testing.T) {
	first := NewID("workflow")
	second := NewID("workflow")
	if first == second || !strings.HasPrefix(first, "workflow-") || !strings.HasPrefix(second, "workflow-") {
		t.Fatalf("unexpected ids: %q %q", first, second)
	}
}

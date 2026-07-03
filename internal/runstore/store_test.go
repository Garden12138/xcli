package runstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadAndList(t *testing.T) {
	store := &Store{root: filepath.Join(t.TempDir(), "runs")}
	record := Record{ID: "run-test", Kind: "run", Agent: "codex", StartedAt: time.Now().UTC(), Status: "success"}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Agent != "codex" {
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

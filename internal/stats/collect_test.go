package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Garden12138/xcli/internal/api"
)

func writeTestFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCollectHomesAndLedgerPriority(t *testing.T) {
	home := t.TempDir()
	first := filepath.Join(home, "chat-insight", "a", "xcli")
	second := filepath.Join(home, "chat-insight", "b", "xcli")
	for _, row := range []struct {
		home string
		id   string
	}{
		{home, "current"},
		{first, "shared"},
		{second, "current"},
	} {
		if err := Append(row.home, Record{V: 1, TaskID: row.id, Status: "completed"}); err != nil {
			t.Fatal(err)
		}
	}
	// Duplicate directories must be skipped even when the ledger is in another home.
	writeTestFile(t, filepath.Join(home, "tasks", "current", "task.json"), []byte("bad content"))
	writeTestFile(t, filepath.Join(home, "tasks", "shared", "task.json"), []byte("bad content"))
	task, events := fixtureTask()
	for i := range events {
		events[i].Seq = i + 1
	}
	writeTask := func(root, id string, status api.Status) {
		task.ID, task.Status = id, status
		raw, err := json.Marshal(task)
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, "tasks", id)
		writeTestFile(t, filepath.Join(dir, "task.json"), raw)
		var lines []byte
		for _, event := range events {
			raw, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			lines = append(append(lines, raw...), '\n')
		}
		writeTestFile(t, filepath.Join(dir, "events.jsonl"), lines)
	}
	writeTask(second, "legacy", api.StatusFailed)
	writeTask(first, "running", api.StatusRunning)
	ledger, err := os.OpenFile(ledgerPath(second), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.WriteString("bad line\n"); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	homes := Homes(home, []string{"chat-insight/*/xcli", second, "missing"})
	if !reflect.DeepEqual(homes, []string{home, first, second}) {
		t.Fatalf("homes = %v", homes)
	}
	records, sources, err := Collect(homes)
	if err != nil || len(records) != 3 || sources.LedgerRecords != 2 || sources.BackfilledTasks != 1 ||
		sources.ApproximateTasks != 1 || sources.SkippedLines != 1 || len(sources.Homes) != 3 {
		t.Fatalf("records = %+v, sources = %+v, err = %v", records, sources, err)
	}
	if legacy := records[2]; legacy.TaskID != "legacy" || !legacy.Approximate || len(legacy.Reviews) != 3 {
		t.Fatalf("backfill = %+v", legacy)
	}
	if info, err := os.ReadFile(filepath.Join(home, "tasks", "current", "task.json")); err != nil || string(info) != "bad content" {
		t.Fatalf("task was modified: %v", err)
	}
}

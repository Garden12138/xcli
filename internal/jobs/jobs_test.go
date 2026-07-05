package jobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/runstore"
)

func TestManagerCreatesPrivateFilesAndReconcilesOrphan(t *testing.T) {
	setEnv(t, "XDG_DATA_HOME", t.TempDir())
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := Manager{Store: store, Now: func() time.Time {
		return time.Date(2026, 7, 5, 1, 2, 3, 0, time.UTC)
	}}
	files, err := manager.CreateFiles("run-job")
	if err != nil {
		t.Fatal(err)
	}
	if held, err := manager.LockHeld("run-job"); err != nil || !held {
		t.Fatalf("held = %v, err = %v", held, err)
	}
	for _, path := range []string{files.LogPath, files.LockPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o", path, info.Mode().Perm())
		}
	}
	files.Log.Close()
	files.Lock.Close()

	record := runstore.Record{
		ID: "run-job", Kind: "run", Agent: "fake", Background: true,
		PID: 123, Cwd: t.TempDir(), StartedAt: time.Now().UTC(), Status: "running",
		LogFile: files.LogPath,
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	loaded, err := manager.Load(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "orphaned" || loaded.ExitCode != 1 || !loaded.EndedAt.Equal(manager.now()) {
		t.Fatalf("unexpected reconciled record: %#v", loaded)
	}
	view := ToView(loaded)
	if view.EndedAt == nil || view.ExitCode == nil || *view.ExitCode != 1 {
		t.Fatalf("unexpected view: %#v", view)
	}
}

func TestManagerRejectsForegroundRecord(t *testing.T) {
	setEnv(t, "XDG_DATA_HOME", t.TempDir())
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(runstore.Record{ID: "run-front", Kind: "run", Cwd: filepath.Clean("/tmp")}); err != nil {
		t.Fatal(err)
	}
	if _, err := (Manager{Store: store}).Load("run-front"); err == nil {
		t.Fatal("expected foreground record error")
	}
}

func TestManagerMarksUnfinishedWorkflowStepsOnOrphanAndKill(t *testing.T) {
	setEnv(t, "XDG_DATA_HOME", t.TempDir())
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := Manager{Store: store, Now: func() time.Time {
		return time.Date(2026, 7, 5, 2, 3, 4, 0, time.UTC)
	}}
	record := runstore.Record{
		ID: "workflow-orphan", Kind: "workflow", Workflow: "test", Background: true,
		Cwd: t.TempDir(), StartedAt: time.Now().UTC(), Status: "running",
		Steps: []runstore.StepRecord{
			{ID: "one", Status: "running"},
			{ID: "two", Status: "pending"},
			{ID: "three", Status: "success"},
		},
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	orphaned, err := manager.Load(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if orphaned.Status != "orphaned" || orphaned.Steps[0].Status != "orphaned" || orphaned.Steps[1].Status != "orphaned" || orphaned.Steps[2].Status != "success" {
		t.Fatalf("unexpected orphaned workflow: %#v", orphaned)
	}

	record.ID = "workflow-killed"
	record.Steps[0].Status = "running"
	record.Steps[1].Status = "pending"
	killed, err := manager.MarkKilled(record)
	if err != nil {
		t.Fatal(err)
	}
	if killed.Status != "killed" || killed.Steps[0].Status != "killed" || killed.Steps[1].Status != "killed" || killed.Steps[2].Status != "success" {
		t.Fatalf("unexpected killed workflow: %#v", killed)
	}
}

func TestManagerDeleteRejectsRunningAndPruneSelectsOldTerminalJobs(t *testing.T) {
	setEnv(t, "XDG_DATA_HOME", t.TempDir())
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := Manager{Store: store}
	files, err := manager.CreateFiles("run-active")
	if err != nil {
		t.Fatal(err)
	}
	defer files.Log.Close()
	defer files.Lock.Close()
	active := runstore.Record{
		ID: "run-active", Kind: "run", Background: true, Status: "running",
		Cwd: t.TempDir(), StartedAt: time.Now().UTC(), LogFile: files.LogPath,
	}
	if err := store.Save(active); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Delete(active.ID); err == nil {
		t.Fatal("expected running job deletion to fail")
	}

	now := time.Now().UTC()
	for _, record := range []runstore.Record{
		{ID: "run-old", Kind: "run", Background: true, Status: "success", StartedAt: now.Add(-3 * time.Hour), EndedAt: now.Add(-2 * time.Hour)},
		{ID: "run-new", Kind: "run", Background: true, Status: "failed", StartedAt: now.Add(-30 * time.Minute), EndedAt: now.Add(-time.Minute)},
		{ID: "run-legacy", Kind: "run", Background: true, Status: "success", StartedAt: now.Add(-3 * time.Hour)},
		{ID: "run-front", Kind: "run", Status: "success", StartedAt: now.Add(-3 * time.Hour), EndedAt: now.Add(-2 * time.Hour)},
	} {
		record.Cwd = t.TempDir()
		if err := store.Save(record); err != nil {
			t.Fatal(err)
		}
	}
	candidates, err := manager.PruneCandidates(now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].ID != "run-old" {
		t.Fatalf("unexpected prune candidates: %#v", candidates)
	}
}

func TestJobDirectoryRejectsPathTraversal(t *testing.T) {
	store := &runstore.Store{}
	for _, id := range []string{"", ".", "..", "../outside", `..\\outside`} {
		if _, err := jobDirectory(store, id); err == nil {
			t.Fatalf("jobDirectory(%q) succeeded", id)
		}
	}
}

func TestManagerDeleteKeepsRecordWhenArtifactRemovalFails(t *testing.T) {
	setEnv(t, "XDG_DATA_HOME", t.TempDir())
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := Manager{Store: store}
	files, err := manager.CreateFiles("run-partial")
	if err != nil {
		t.Fatal(err)
	}
	files.Log.Close()
	files.Lock.Close()
	now := time.Now().UTC()
	if err := store.Save(runstore.Record{
		ID: "run-partial", Kind: "run", Background: true, Status: "success",
		Cwd: t.TempDir(), StartedAt: now.Add(-time.Minute), EndedAt: now, LogFile: files.LogPath,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.Root(), 0o500); err != nil {
		t.Fatal(err)
	}
	_, deleteErr := manager.Delete("run-partial")
	if err := os.Chmod(store.Root(), 0o700); err != nil {
		t.Fatal(err)
	}
	if deleteErr == nil {
		t.Skip("filesystem permissions did not prevent artifact removal")
	}
	if _, err := store.Load("run-partial"); err != nil {
		t.Fatalf("record was not retained after partial deletion failure: %v", err)
	}
}

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	old, existed := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

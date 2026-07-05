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

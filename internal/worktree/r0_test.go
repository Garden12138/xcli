package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlannedPathAndStartupRejectEscapes(t *testing.T) {
	repo := initTestRepository(t)
	manager := NewAt(filepath.Join(t.TempDir(), "managed"))
	preview, err := manager.Preview("preview", repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(preview.Path); !os.IsNotExist(err) {
		t.Fatal("preview created a worktree")
	}
	outside := t.TempDir()
	for _, path := range []string{outside, filepath.Join(repo, ".."), filepath.Join(repo, "missing")} {
		if _, err := PlannedPath(preview, path); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
	link := filepath.Join(repo, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := PlannedPath(preview, link); err == nil {
		t.Fatal("accepted original symlink escape")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	created, err := manager.Create("actual", repo)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, original := range []string{repo, filepath.Join(repo, "sub"), filepath.Join(canonicalRepo, "sub")} {
		expected, err := PlannedPath(created, original)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := manager.MapPath(created, original)
		if err != nil || actual != expected {
			t.Fatalf("%s != %s: %v", actual, expected, err)
		}
	}
	target := filepath.Join(created.Path, "sub")
	if err := os.Rename(target, filepath.Join(created.Path, "saved-sub")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.MapPath(created, filepath.Join(repo, "sub")); err == nil {
		t.Fatal("mapped an escaped worktree symlink")
	}
	if err := CheckDirectory(created.Path, target); err == nil {
		t.Fatal("startup accepted replaced directory")
	}
}

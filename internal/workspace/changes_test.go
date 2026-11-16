package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/api"
)

func TestChangesGitExcludesPreexisting(t *testing.T) {
	repo := newRepo(t)
	writeTest(t, filepath.Join(repo, "a.go"), "package demo\nvar A = 2\n")
	writeTest(t, filepath.Join(repo, "old.txt"), "old")
	w, _, err := Prepare(t.TempDir(), "t-test", api.Request{Workdir: repo})
	if err != nil {
		t.Fatal(err)
	}
	indexBefore := gitTest(t, repo, "diff", "--cached")
	if err := w.Baseline(); err != nil {
		t.Fatal(err)
	}
	writeTest(t, filepath.Join(repo, "b.go"), "package demo\nvar B = 2\n")
	writeTest(t, filepath.Join(repo, "new.txt"), "new")
	patch := filepath.Join(t.TempDir(), "changes.patch")
	c, err := w.Changes(patch)
	if err != nil || !reflect.DeepEqual(c.Files, []string{"b.go", "new.txt"}) {
		t.Fatalf("changes: %+v, %v", c, err)
	}
	data, err := os.ReadFile(patch)
	if err != nil || !strings.Contains(string(data), "b.go") || !strings.Contains(string(data), "new.txt") || strings.Contains(string(data), "a.go") || strings.Contains(string(data), "old.txt") {
		t.Fatalf("patch: %s, %v", data, err)
	}
	if after := gitTest(t, repo, "diff", "--cached"); after != indexBefore {
		t.Fatal("change detection modified the index")
	}
}

func TestChangesNonGit(t *testing.T) {
	dir := t.TempDir()
	writeTest(t, filepath.Join(dir, "a.txt"), "before")
	w, _, err := Prepare(t.TempDir(), "t-test", api.Request{Workdir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Baseline(); err != nil {
		t.Fatal(err)
	}
	writeTest(t, filepath.Join(dir, "a.txt"), "after longer")
	writeTest(t, filepath.Join(dir, "b.txt"), "new")
	c, err := w.Changes(filepath.Join(t.TempDir(), "unused.patch"))
	if err != nil || !reflect.DeepEqual(c.Files, []string{"a.txt", "b.txt"}) || c.DiffFile != "" {
		t.Fatalf("non-git changes: %+v, %v", c, err)
	}
}

func TestBaselineRemainsAcrossSteps(t *testing.T) {
	dir := newRepo(t)
	w, _, err := Prepare(t.TempDir(), "t-test", api.Request{Workdir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Baseline(); err != nil {
		t.Fatal(err)
	}
	writeTest(t, filepath.Join(dir, "first.txt"), "first step")
	if err := w.Baseline(); err != nil {
		t.Fatal(err)
	}
	writeTest(t, filepath.Join(dir, "second.txt"), "second step")
	c, err := w.Changes(filepath.Join(t.TempDir(), "all.patch"))
	if err != nil || !reflect.DeepEqual(c.Files, []string{"first.txt", "second.txt"}) {
		t.Fatalf("task baseline: %+v, %v", c, err)
	}
}

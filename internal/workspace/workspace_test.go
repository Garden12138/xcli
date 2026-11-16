package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/api"
)

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s, %v", args, raw, err)
	}
	return string(raw)
}

func writeTest(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for key, value := range map[string]string{"GIT_AUTHOR_NAME": "xcli-test", "GIT_AUTHOR_EMAIL": "test@localhost", "GIT_COMMITTER_NAME": "xcli-test", "GIT_COMMITTER_EMAIL": "test@localhost"} {
		t.Setenv(key, value)
	}
	dir := t.TempDir()
	gitTest(t, dir, "init", "-q")
	writeTest(t, filepath.Join(dir, "a.go"), "package demo\nvar A = 1\n")
	writeTest(t, filepath.Join(dir, "b.go"), "package demo\nvar B = 1\n")
	gitTest(t, dir, "add", ".")
	gitTest(t, dir, "commit", "-qm", "init")
	return dir
}

func TestPrepareTemp(t *testing.T) {
	task := t.TempDir()
	w, _, err := Prepare(task, "t-test", api.Request{})
	if err != nil || w.Dir != filepath.Join(task, "work") || w.Isolation != "temp" || w.Git {
		t.Fatalf("temp: %+v, %v", w, err)
	}
}

func TestPrepareInPlace(t *testing.T) {
	repo := newRepo(t)
	w, _, err := Prepare(t.TempDir(), "t-test", api.Request{Workdir: repo})
	if err != nil || w.Dir != repo || w.Original != repo || w.Isolation != "none" || !w.Git {
		t.Fatalf("in place: %+v, %v", w, err)
	}
}

func TestPrepareWorktree(t *testing.T) {
	repo, task := newRepo(t), t.TempDir()
	writeTest(t, filepath.Join(repo, "a.go"), "dirty\n")
	w, notices, err := Prepare(task, "t-test", api.Request{Workdir: repo, Isolate: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RemoveWorktree(repo, w.Dir, w.Branch) })
	if w.Dir != filepath.Join(task, "worktree") || w.Branch != "xcli/t-test" || w.Isolation != "worktree" || !strings.Contains(strings.Join(notices, " "), "未提交的改动不会带入") {
		t.Fatalf("worktree: %+v, %v", w, notices)
	}
	if text, _ := os.ReadFile(filepath.Join(w.Dir, "a.go")); string(text) == "dirty\n" {
		t.Fatal("worktree carried uncommitted changes")
	}
	gitTest(t, repo, "rev-parse", "--verify", w.Branch)
}

func TestPrepareWorktreeNonGit(t *testing.T) {
	if _, _, err := Prepare(t.TempDir(), "t-test", api.Request{Workdir: t.TempDir(), Isolate: true}); err == nil || !strings.Contains(err.Error(), "isolate 需要 git 仓库") {
		t.Fatalf("non-git isolation: %v", err)
	}
}

func TestSnapshotInputs(t *testing.T) {
	source, task := t.TempDir(), t.TempDir()
	writeTest(t, filepath.Join(source, "note.txt"), "before")
	w, _, err := Prepare(task, "t-test", api.Request{Inputs: []string{source}, Images: []string{filepath.Join(source, "note.txt")}})
	if err != nil || len(w.Inputs) != 1 || len(w.Images) != 1 || !filepath.IsAbs(w.Inputs[0]) {
		t.Fatalf("snapshot: %+v, %v", w, err)
	}
	writeTest(t, filepath.Join(source, "note.txt"), "after")
	if data, _ := os.ReadFile(filepath.Join(w.Inputs[0], "note.txt")); string(data) != "before" {
		t.Fatalf("directory snapshot changed: %s", data)
	}
	if data, _ := os.ReadFile(w.Images[0]); string(data) != "before" {
		t.Fatalf("image snapshot changed: %s", data)
	}
}

func TestSnapshotInputsLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.bin")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(201 * 1024 * 1024); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()
	if _, _, err := Prepare(t.TempDir(), "t-test", api.Request{Inputs: []string{path}}); err == nil || !strings.Contains(err.Error(), "200 MB") {
		t.Fatalf("snapshot limit: %v", err)
	}
}

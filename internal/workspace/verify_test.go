package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/api"
)

func TestRunVerify(t *testing.T) {
	logDir := t.TempDir()
	results := RunVerify(context.Background(), t.TempDir(), []string{"true", "echo out; echo err >&2; exit 3"}, time.Second, logDir, 2)
	if len(results) != 2 || !results[0].Passed || results[1].Passed || results[1].ExitCode != 3 || results[1].Round != 2 {
		t.Fatalf("verify: %+v", results)
	}
	data, err := os.ReadFile(results[1].LogFile)
	if err != nil || !strings.Contains(string(data), "out") || !strings.Contains(string(data), "err") {
		t.Fatalf("combined log: %s, %v", data, err)
	}
	results = RunVerify(context.Background(), t.TempDir(), []string{"sleep 5"}, 200*time.Millisecond, logDir, 3)
	if results[0].Passed {
		t.Fatal("timeout passed verification")
	}
}

func TestApply(t *testing.T) {
	repo := newRepo(t)
	gitTest(t, repo, "config", "color.ui", "always")
	w, _, err := Prepare(t.TempDir(), "t-apply", api.Request{Workdir: repo, Isolate: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RemoveWorktree(repo, w.Dir, w.Branch) })
	if err := w.Baseline(); err != nil {
		t.Fatal(err)
	}
	writeTest(t, filepath.Join(w.Dir, "a.go"), "package demo\nvar A = 42\n")
	writeTest(t, filepath.Join(w.Dir, "new.txt"), "new artifact")
	patch := filepath.Join(t.TempDir(), "changes.patch")
	if _, err := w.Changes(patch); err != nil {
		t.Fatal(err)
	}
	if err := Apply(patch, repo); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(repo, "a.go")); !strings.Contains(string(data), "42") {
		t.Fatalf("applied content: %s", data)
	}
	if data, _ := os.ReadFile(filepath.Join(repo, "new.txt")); string(data) != "new artifact" {
		t.Fatalf("new file: %s", data)
	}
}

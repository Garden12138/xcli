package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/runstore"
	"github.com/Garden12138/xcli/internal/worktree"
)

func TestWorktreesCommandsEnforceTerminalAndDirtySafety(t *testing.T) {
	directory := t.TempDir()
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	repository := filepath.Join(directory, "repo")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	runCommandGit(t, repository, "init")
	runCommandGit(t, repository, "config", "user.name", "xcli tests")
	runCommandGit(t, repository, "config", "user.email", "xcli@example.invalid")
	if err := os.WriteFile(filepath.Join(repository, "tracked.txt"), []byte("tracked"), 0o600); err != nil {
		t.Fatal(err)
	}
	runCommandGit(t, repository, "add", ".")
	runCommandGit(t, repository, "commit", "-m", "initial")

	manager, err := worktree.New()
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := manager.Create("run-worktree", repository)
	if err != nil {
		t.Fatal(err)
	}
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	record := runstore.Record{
		ID: metadata.RunID, Kind: "run", Agent: "codex", Cwd: metadata.Path,
		Status: "running", WorkspaceMode: "worktree", WorktreePath: metadata.Path,
		WorktreeBranch: metadata.Branch, BaseCommit: metadata.BaseCommit,
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"worktrees", "list", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var listed []worktreeView
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].RunID != metadata.RunID || listed[0].Status != "running" {
		t.Fatalf("unexpected worktree list: %#v", listed)
	}

	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"worktrees", "show", metadata.RunID})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var shown worktreeView
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatal(err)
	}
	if shown.RunID != metadata.RunID || shown.Branch != metadata.Branch {
		t.Fatalf("unexpected worktree: %#v", shown)
	}

	root = newRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"worktrees", "remove", metadata.RunID, "--yes"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "still running") {
		t.Fatalf("expected running refusal, got %v", err)
	}

	record.Status = "success"
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata.Path, "dirty.txt"), []byte("dirty"), 0o600); err != nil {
		t.Fatal(err)
	}
	root = newRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"worktrees", "remove", metadata.RunID, "--yes"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("expected dirty refusal, got %v", err)
	}

	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"worktrees", "remove", metadata.RunID, "--force", "--yes", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["retained_branch"] != metadata.Branch {
		t.Fatalf("unexpected removal result: %#v", result)
	}
	runCommandGit(t, repository, "show-ref", "--verify", "refs/heads/"+metadata.Branch)
}

func runCommandGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-C", directory}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

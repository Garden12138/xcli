package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateMapListAndSafelyRemoveWorktree(t *testing.T) {
	repository := initTestRepository(t)
	root := filepath.Join(t.TempDir(), "managed")
	manager := NewAt(root)
	manager.now = func() time.Time {
		return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	}

	metadata, err := manager.Create("run-test", filepath.Join(repository, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	canonicalRepository, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Repository != canonicalRepository || metadata.Branch != "xcli/run-test" ||
		metadata.BaseCommit == "" || metadata.CreatedAt.IsZero() {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	mapped, err := manager.MapPath(metadata, filepath.Join(repository, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if mapped != filepath.Join(metadata.Path, "sub") {
		t.Fatalf("mapped path = %q", mapped)
	}
	info, err := os.Stat(filepath.Join(root, "run-test.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("metadata mode = %o, want 600", info.Mode().Perm())
	}
	listed, err := manager.List()
	if err != nil || len(listed) != 1 || listed[0].RunID != metadata.RunID {
		t.Fatalf("list = %#v, %v", listed, err)
	}

	dirty := filepath.Join(metadata.Path, "uncommitted.txt")
	if err := os.WriteFile(dirty, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(metadata, false); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("expected dirty-worktree refusal, got %v", err)
	}
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(metadata, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(metadata.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists: %v", err)
	}
	runGit(t, repository, "show-ref", "--verify", "refs/heads/"+metadata.Branch)
	if _, err := manager.Load(metadata.RunID); !os.IsNotExist(err) {
		t.Fatalf("metadata was not removed: %v", err)
	}
}

func TestPreflightRejectsDirtyRepositoryAndForceRemovalDiscardsChanges(t *testing.T) {
	repository := initTestRepository(t)
	manager := NewAt(filepath.Join(t.TempDir(), "managed"))
	tracked := filepath.Join(repository, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("dirty"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Preflight(repository); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("expected dirty-repository error, got %v", err)
	}
	runGit(t, repository, "checkout", "--", "tracked.txt")

	metadata, err := manager.Create("run-force", repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata.Path, "discard.txt"), []byte("discard"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(metadata, true); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "show-ref", "--verify", "refs/heads/"+metadata.Branch)
}

func TestCreateRollsBackWhenCwdIsNotPresentAtBaseCommit(t *testing.T) {
	repository := initTestRepository(t)
	emptyDirectory := filepath.Join(repository, "empty-untracked-directory")
	if err := os.MkdirAll(emptyDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	manager := NewAt(filepath.Join(t.TempDir(), "managed"))
	if _, err := manager.Create("run-rollback", emptyDirectory); err == nil ||
		!strings.Contains(err.Error(), "isolated working directory") {
		t.Fatalf("expected mapping failure, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(manager.Root(), "run-rollback")); !os.IsNotExist(err) {
		t.Fatalf("rolled-back worktree still exists: %v", err)
	}
	command := exec.Command("git", "-C", repository, "show-ref", "--verify", "--quiet", "refs/heads/xcli/run-rollback")
	if err := command.Run(); err == nil {
		t.Fatal("rolled-back branch still exists")
	}
}

func TestForceRemoveCleansStaleMetadata(t *testing.T) {
	repository := initTestRepository(t)
	manager := NewAt(filepath.Join(t.TempDir(), "managed"))
	metadata, err := manager.Create("run-stale", repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(metadata.Path); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(metadata, false); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected stale-path refusal, got %v", err)
	}
	if err := manager.Remove(metadata, true); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Load(metadata.RunID); !os.IsNotExist(err) {
		t.Fatalf("stale metadata still exists: %v", err)
	}
}

func TestRunsReceiveDistinctWorktreesAndBranches(t *testing.T) {
	repository := initTestRepository(t)
	manager := NewAt(filepath.Join(t.TempDir(), "managed"))
	first, err := manager.Create("run-one", repository)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Create("run-two", repository)
	if err != nil {
		t.Fatal(err)
	}
	if first.Path == second.Path || first.Branch == second.Branch ||
		first.BaseCommit != second.BaseCommit {
		t.Fatalf("worktrees are not isolated: first=%#v second=%#v", first, second)
	}
	if err := manager.Remove(first, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(second, false); err != nil {
		t.Fatal(err)
	}
}

func TestIgnoredFilesAreNotCopiedIntoWorktree(t *testing.T) {
	repository := initTestRepository(t)
	if err := os.WriteFile(filepath.Join(repository, ".gitignore"), []byte(".env\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", ".gitignore")
	runGit(t, repository, "commit", "-m", "ignore environment")
	if err := os.WriteFile(filepath.Join(repository, ".env"), []byte("SECRET=not-copied"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewAt(filepath.Join(t.TempDir(), "managed"))
	metadata, err := manager.Create("run-ignore", repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(metadata.Path, ".env")); !os.IsNotExist(err) {
		t.Fatalf("ignored environment file was copied: %v", err)
	}
	if err := manager.Remove(metadata, false); err != nil {
		t.Fatal(err)
	}
}

func initTestRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	runGit(t, repository, "init")
	runGit(t, repository, "config", "user.name", "xcli tests")
	runGit(t, repository, "config", "user.email", "xcli@example.invalid")
	if err := os.MkdirAll(filepath.Join(repository, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "tracked.txt"), []byte("clean"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "sub", "tracked.txt"), []byte("clean"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", ".")
	runGit(t, repository, "commit", "-m", "initial")
	absolute, err := filepath.Abs(repository)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func runGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-C", directory}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

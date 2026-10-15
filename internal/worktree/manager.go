package worktree

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/runstore"
)

type Metadata struct {
	RunID      string    `json:"run_id"`
	Repository string    `json:"repository"`
	Path       string    `json:"path"`
	Branch     string    `json:"branch"`
	BaseCommit string    `json:"base_commit"`
	CreatedAt  time.Time `json:"created_at"`
}

type Manager struct {
	root string
	now  func() time.Time
}

func New() (*Manager, error) {
	root, err := runstore.DataPath()
	if err != nil {
		return nil, err
	}
	root, err = canonicalPath(filepath.Join(root, "worktrees"))
	if err != nil {
		return nil, err
	}
	return &Manager{root: root}, nil
}

func NewAt(root string) *Manager {
	if canonical, err := canonicalPath(root); err == nil {
		root = canonical
	}
	return &Manager{root: root}
}

func (m *Manager) Root() string {
	return m.root
}

func (m *Manager) Create(runID, cwd string) (Metadata, error) {
	if err := validateRunID(runID); err != nil {
		return Metadata{}, err
	}
	repository, baseCommit, err := m.Preflight(cwd)
	if err != nil {
		return Metadata{}, err
	}
	branch := "xcli/" + runID
	if err := ensureBranchMissing(repository, branch); err != nil {
		return Metadata{}, err
	}
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return Metadata{}, fmt.Errorf("create worktree data directory: %w", err)
	}
	path := filepath.Join(m.root, runID)
	if _, err := os.Stat(path); err == nil {
		return Metadata{}, fmt.Errorf("worktree path already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Metadata{}, err
	}
	if _, err := gitOutput(repository, "worktree", "add", "-b", branch, path, baseCommit); err != nil {
		return Metadata{}, fmt.Errorf("create Git worktree: %w", err)
	}
	metadata := Metadata{
		RunID: runID, Repository: repository, Path: path, Branch: branch,
		BaseCommit: baseCommit, CreatedAt: m.currentTime().UTC(),
	}
	if _, err := m.MapPath(metadata, cwd); err != nil {
		return Metadata{}, m.rollbackCreate(metadata, err)
	}
	if err := m.save(metadata); err != nil {
		return Metadata{}, m.rollbackCreate(metadata, err)
	}
	return metadata, nil
}

func (m *Manager) Preflight(cwd string) (string, string, error) {
	repository, err := gitOutput(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("resolve Git repository: %w", err)
	}
	repository, err = canonicalExistingPath(repository)
	if err != nil {
		return "", "", fmt.Errorf("resolve Git repository path: %w", err)
	}
	status, err := gitOutput(repository, "status", "--porcelain")
	if err != nil {
		return "", "", fmt.Errorf("inspect Git worktree: %w", err)
	}
	if status != "" {
		return "", "", errors.New("cannot create an isolated worktree from a dirty repository; commit or stash changes first")
	}
	baseCommit, err := gitOutput(repository, "rev-parse", "HEAD")
	if err != nil {
		return "", "", fmt.Errorf("resolve Git HEAD: %w", err)
	}
	return repository, baseCommit, nil
}

func (m *Manager) MapPath(metadata Metadata, original string) (string, error) {
	absolute, err := canonicalExistingPath(original)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(metadata.Repository, absolute)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("working directory %s is outside repository %s", absolute, metadata.Repository)
	}
	mapped := filepath.Join(metadata.Path, relative)
	canonical, err := canonicalExistingPath(mapped)
	if err != nil {
		return "", fmt.Errorf("isolated working directory: %w", err)
	}
	root, err := canonicalExistingPath(metadata.Path)
	if err != nil {
		return "", err
	}
	inside, err := filepath.Rel(root, canonical)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("isolated working directory escapes worktree: %s", mapped)
	}
	info, err := os.Stat(mapped)
	if err != nil {
		return "", fmt.Errorf("isolated working directory %s: %w", mapped, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("isolated working directory %s is not a directory", mapped)
	}
	return canonical, nil
}

// Preview resolves a stable worktree identity without creating files or Git
// branches. Prepare and execution use the same metadata and directory map.
func (m *Manager) Preview(runID, cwd string) (Metadata, error) {
	if err := validateRunID(runID); err != nil {
		return Metadata{}, err
	}
	repo, base, err := m.Preflight(cwd)
	if err != nil {
		return Metadata{}, err
	}
	return Metadata{RunID: runID, Repository: repo, BaseCommit: base, Path: filepath.Join(m.root, runID), Branch: "xcli/" + runID}, nil
}

func PlannedPath(metadata Metadata, original string) (string, error) {
	original, err := canonicalExistingPath(original)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(metadata.Repository, original)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("working directory is outside repository: %s", original)
	}
	return filepath.Join(metadata.Path, rel), nil
}

func CheckDirectory(root, path string) error {
	expected, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	root, err = canonicalExistingPath(root)
	if err != nil {
		return err
	}
	path, err = canonicalExistingPath(path)
	if err != nil {
		return err
	}
	if path != expected {
		return fmt.Errorf("working directory changed after preparation: %s", expected)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("working directory escapes worktree: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("working directory is not a directory: %s", path)
	}
	return nil
}

func canonicalExistingPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := absolute
	missing := []string{}
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("resolve existing ancestor for %s", absolute)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	for index := len(missing) - 1; index >= 0; index-- {
		current = filepath.Join(current, missing[index])
	}
	return current, nil
}

func (m *Manager) Load(runID string) (Metadata, error) {
	if err := validateRunID(runID); err != nil {
		return Metadata{}, err
	}
	data, err := os.ReadFile(m.metadataPath(runID))
	if err != nil {
		return Metadata{}, err
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Metadata{}, fmt.Errorf("decode worktree metadata: %w", err)
	}
	if metadata.RunID != runID {
		return Metadata{}, fmt.Errorf("worktree metadata belongs to %q", metadata.RunID)
	}
	if err := m.validateManagedPath(metadata.Path); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

func (m *Manager) List() ([]Metadata, error) {
	entries, err := os.ReadDir(m.root)
	if errors.Is(err, os.ErrNotExist) {
		return []Metadata{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]Metadata, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		runID := strings.TrimSuffix(entry.Name(), ".json")
		metadata, err := m.Load(runID)
		if err == nil {
			result = append(result, metadata)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].RunID < result[j].RunID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func (m *Manager) Remove(metadata Metadata, force bool) error {
	if err := m.validateManagedPath(metadata.Path); err != nil {
		return err
	}
	if _, err := os.Stat(metadata.Path); errors.Is(err, os.ErrNotExist) {
		if !force {
			return errors.New("isolated worktree path is missing; pass --force to remove stale metadata")
		}
		_, _ = gitOutput(metadata.Repository, "worktree", "prune")
		if err := os.Remove(m.metadataPath(metadata.RunID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale worktree metadata: %w", err)
		}
		return nil
	} else if err != nil {
		return err
	}
	status, err := gitOutput(metadata.Path, "status", "--porcelain", "--untracked-files=all", "--ignored=matching")
	if err != nil {
		return fmt.Errorf("inspect isolated worktree: %w", err)
	}
	if status != "" && !force {
		return errors.New("isolated worktree has uncommitted changes; commit them or pass --force")
	}
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, metadata.Path)
	if _, err := gitOutput(metadata.Repository, args...); err != nil {
		return fmt.Errorf("remove Git worktree: %w", err)
	}
	if err := os.Remove(m.metadataPath(metadata.RunID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove worktree metadata: %w", err)
	}
	return nil
}

func (m *Manager) rollbackCreate(metadata Metadata, cause error) error {
	_, removeErr := gitOutput(metadata.Repository, "worktree", "remove", "--force", metadata.Path)
	_, branchErr := gitOutput(metadata.Repository, "branch", "-D", metadata.Branch)
	if removeErr != nil || branchErr != nil {
		return fmt.Errorf(
			"%w (rollback worktree: %v; rollback branch: %v)",
			cause, removeErr, branchErr,
		)
	}
	return cause
}

func (m *Manager) save(metadata Metadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(m.root, ".xcli-worktree-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, m.metadataPath(metadata.RunID))
}

func (m *Manager) metadataPath(runID string) string {
	return filepath.Join(m.root, runID+".json")
}

func (m *Manager) validateManagedPath(path string) error {
	absoluteRoot, err := canonicalPath(m.root)
	if err != nil {
		return err
	}
	absolutePath, err := canonicalPath(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(absoluteRoot, absolutePath)
	if err != nil {
		return err
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("worktree path %s is outside managed root %s", absolutePath, absoluteRoot)
	}
	return nil
}

func (m *Manager) currentTime() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

func validateRunID(runID string) error {
	if runID == "" || filepath.Base(runID) != runID || strings.ContainsAny(runID, `/\\`) || runID == "." || runID == ".." {
		return errors.New("invalid run id")
	}
	return nil
}

func ensureBranchMissing(repository, branch string) error {
	command := exec.Command("git", "-C", repository, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	err := command.Run()
	if err == nil {
		return fmt.Errorf("Git branch %q already exists", branch)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("check Git branch %q: %w", branch, err)
}

func gitOutput(directory string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", directory}, args...)
	command := exec.Command("git", commandArgs...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return "", fmt.Errorf("%s: %w", detail, err)
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

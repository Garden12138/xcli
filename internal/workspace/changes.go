package workspace

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Garden12138/xcli/internal/api"
)

type fileState struct{ size, mtime int64 }

func snapshot(dir string) (map[string]fileState, error) {
	files := map[string]fileState{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && (d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == ".venv" || d.Name() == "dist" || d.Name() == "build") {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= 20000 {
			return errors.New("目录超过 20000 个文件，请缩小 workdir 范围后重试")
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err == nil {
			files[rel] = fileState{info.Size(), info.ModTime().UnixNano()}
		}
		return err
	})
	return files, err
}

func (w *Workspace) untrackedFiles() (map[string]bool, error) {
	raw, err := gitOutput(w.Dir, "ls-files", "--others", "--exclude-standard", "-z")
	files := map[string]bool{}
	for _, path := range strings.Split(string(raw), "\x00") {
		if path != "" {
			files[path] = true
		}
	}
	return files, err
}

func (w *Workspace) Baseline() error {
	w.once.Do(func() {
		if !w.Git {
			w.files, w.baseErr = snapshot(w.Dir)
			return
		}
		var raw []byte
		raw, w.baseErr = gitOutput(w.Dir, "-c", "user.name=xcli", "-c", "user.email=xcli@localhost", "-c", "commit.gpgsign=false", "stash", "create")
		if w.baseErr != nil {
			return
		}
		w.base = strings.TrimSpace(string(raw))
		if w.base == "" {
			raw, w.baseErr = gitOutput(w.Dir, "rev-parse", "HEAD")
			w.base = strings.TrimSpace(string(raw))
		}
		if w.baseErr == nil {
			w.untracked, w.baseErr = w.untrackedFiles()
		}
	})
	return w.baseErr
}

func (w *Workspace) Changes(patchPath string) (*api.Changes, error) {
	if err := w.Baseline(); err != nil {
		return nil, err
	}
	changes := &api.Changes{Workdir: w.Dir, Isolation: w.Isolation, Branch: w.Branch, Files: []string{}}
	if changes.Isolation == "temp" {
		changes.Isolation = "none"
	}
	files := map[string]bool{}
	if !w.Git {
		now, err := snapshot(w.Dir)
		if err != nil {
			return nil, err
		}
		for name, state := range now {
			if old, ok := w.files[name]; !ok || old != state {
				files[name] = true
			}
		}
		for name := range w.files {
			if _, ok := now[name]; !ok {
				files[name] = true
			}
		}
	} else {
		patch, err := gitOutput(w.Dir, "diff", "--binary", w.base, "--")
		if err != nil {
			return nil, err
		}
		names, err := gitOutput(w.Dir, "diff", "--name-only", "-z", w.base, "--")
		if err != nil {
			return nil, err
		}
		for _, name := range bytes.Split(names, []byte{0}) {
			if len(name) > 0 {
				files[string(name)] = true
			}
		}
		untracked, err := w.untrackedFiles()
		if err != nil {
			return nil, err
		}
		for name := range untracked {
			if w.untracked[name] {
				continue
			}
			raw, err := gitOutput(w.Dir, "diff", "--no-index", "--binary", "--", "/dev/null", name)
			var exit *exec.ExitError
			if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
				return nil, err
			}
			patch = append(patch, raw...)
			files[name] = true
		}
		if err := os.WriteFile(patchPath, patch, 0600); err != nil {
			return nil, err
		}
		changes.DiffFile = patchPath
	}
	for name := range files {
		changes.Files = append(changes.Files, name)
	}
	sort.Strings(changes.Files)
	return changes, nil
}

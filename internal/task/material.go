package task

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const maxFile = 32 << 20
const maxMaterial = 128 << 20

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func canonical(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func boundedRead(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxFile {
		return nil, fmt.Errorf("material is not a regular file of at most 32 MiB: %s", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if len(data) > maxFile {
		return nil, errors.New("material grew beyond 32 MiB")
	}
	return data, err
}

func git(cwd string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	var out limitedWriter
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	if out.exceeded {
		return nil, errors.New("Git evidence exceeds 128 MiB")
	}
	return out.data, nil
}

type limitedWriter struct {
	data     []byte
	exceeded bool
}

func (w *limitedWriter) Write(b []byte) (int, error) {
	n := len(b)
	if len(w.data)+n > maxMaterial {
		w.exceeded = true
		b = b[:max(0, maxMaterial-len(w.data))]
	}
	w.data = append(w.data, b...)
	return n, nil
}

// files enumerates tracked and non-ignored untracked content for Git projects;
// ignored build/cache files cannot silently replace source in the evidence.
func files(root string, project bool) ([]string, error) {
	var names []string
	if project {
		data, err := git(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
		if err != nil {
			return nil, err
		}
		for _, name := range strings.Split(string(data), "\x00") {
			if name != "" {
				names = append(names, name)
			}
		}
	} else {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			names = append(names, rel)
			if len(names) > 10000 {
				return errors.New("too many material files")
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(names)
	return names, nil
}

func material(root string, project bool, destination string) ([]Artifact, string, error) {
	names, err := files(root, project)
	if err != nil {
		return nil, "", err
	}
	result := make([]Artifact, 0, len(names))
	total := 0
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		path := filepath.Join(root, name)
		if !inside(root, path) {
			return nil, "", errors.New("material path escapes root")
		}
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) && project { // tracked deletion is included in the manifest
			result = append(result, Artifact{Path: name, SHA256: "deleted"})
			continue
		}
		if err != nil {
			return nil, "", err
		}
		if !info.Mode().IsRegular() {
			return nil, "", fmt.Errorf("material contains symlink or special file: %s", name)
		}
		data, err := boundedRead(path)
		if err != nil {
			return nil, "", err
		}
		total += len(data)
		if total > maxMaterial {
			return nil, "", errors.New("task material exceeds 128 MiB")
		}
		result = append(result, Artifact{Path: name, SHA256: digest(data), Mode: uint32(info.Mode().Perm())})
		if destination != "" {
			target := filepath.Join(destination, name)
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return nil, "", err
			}
			if err := os.WriteFile(target, data, 0o600); err != nil {
				return nil, "", err
			}
		}
	}
	data, _ := json.Marshal(result)
	return result, digest(data), nil
}

func requirementPath(root, name string) (string, error) {
	if name == "" || filepath.IsAbs(name) || name == "." || !inside(root, filepath.Join(root, name)) {
		return "", fmt.Errorf("invalid artifact path %q", name)
	}
	path, err := canonical(filepath.Join(root, name))
	if err != nil {
		return "", err
	}
	if !inside(root, path) {
		return "", errors.New("artifact symlink escapes output")
	}
	return path, nil
}

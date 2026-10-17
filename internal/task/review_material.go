package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// reviewMaterial places selected source and verification evidence inside the
// fixed copy. Reviewers can inspect actual logs and baseline diff without
// permission to read the live workspace or unrelated private Task records.
func (r *Runner) reviewMaterial(dir string) (string, error) {
	evidence := filepath.Join(dir, ".xcli-task-evidence")
	if err := os.Mkdir(evidence, 0o700); err != nil {
		return "", fmt.Errorf("create review evidence: %w", err)
	}
	sources := map[string]string{}
	copyFile := func(source, name string) (string, error) {
		data, err := boundedRead(source)
		if err != nil {
			return "", err
		}
		path := filepath.Join(evidence, name)
		return path, os.WriteFile(path, data, 0o600)
	}
	for i, a := range r.task.Inputs {
		data, err := boundedRead(a.Path)
		if err != nil {
			return "", err
		}
		if digest(data) != a.SHA256 {
			return "", errors.New("input changed before review")
		}
		path := filepath.Join(evidence, fmt.Sprintf("source-%d-%s", i, filepath.Base(a.Path)))
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return "", err
		}
		sources[a.Path] = path
	}
	checks := append([]Check(nil), r.task.Checks...)
	for i := range checks {
		if checks[i].LogFile != "" {
			path, err := copyFile(checks[i].LogFile, fmt.Sprintf("check-%d.txt", i))
			if err != nil {
				return "", err
			}
			checks[i].LogFile = path
		}
	}
	var brief map[string]any
	if err := json.Unmarshal([]byte(r.brief()), &brief); err != nil {
		return "", err
	}
	brief["checks"] = checks
	brief["input_snapshots"] = sources
	if r.task.Workspace != nil {
		path, err := copyFile(r.task.DiffFile, "changes.patch")
		if err != nil {
			return "", err
		}
		brief["diff_file"] = path
		brief["workspace"] = r.task.Workspace
	}
	return compactJSON(brief), nil
}

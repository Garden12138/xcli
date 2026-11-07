package agentstore

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/Garden12138/xcli/internal/modelstore"
)

var objectID = regexp.MustCompile(`^(ses|tsk|inp|art)_[a-f0-9]{32}$`)

func (s *Store) Workspace(sessionID, taskID string) (string, string, error) {
	if !objectID.MatchString(sessionID) || !objectID.MatchString(taskID) {
		return "", "", ErrUnavailable
	}
	base := s.Root
	for _, component := range []string{"workspaces", sessionID} {
		base = filepath.Join(base, component)
		if e := modelstore.PrivateDirectory(base); e != nil {
			return "", "", e
		}
	}
	workspace := base
	for _, component := range []string{"output", taskID} {
		base = filepath.Join(base, component)
		if e := modelstore.PrivateDirectory(base); e != nil {
			return "", "", e
		}
	}
	return workspace, base, nil
}
func (s *Store) RemoveWorkspace(sessionID string) error {
	if !objectID.MatchString(sessionID) {
		return ErrUnavailable
	}
	return os.RemoveAll(filepath.Join(s.Root, "workspaces", sessionID))
}

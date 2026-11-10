// Package localstate owns the shared private-file and OS-lock primitives.
package localstate

import (
	"errors"
	"os"
	"path/filepath"
)

var ErrLocked = errors.New("state is locked by another process")

func DataPath() (string, error) {
	if p := os.Getenv("XDG_DATA_HOME"); p != "" {
		return filepath.Join(p, "xcli"), nil
	}
	h, e := os.UserHomeDir()
	return filepath.Join(h, ".local", "share", "xcli"), e
}

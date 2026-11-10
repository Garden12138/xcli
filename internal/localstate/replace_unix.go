//go:build darwin || linux

package localstate

import (
	"os"
	"path/filepath"
)

func replaceDurable(from, to string) error {
	if e := os.Rename(from, to); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(to))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}

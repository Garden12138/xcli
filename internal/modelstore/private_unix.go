//go:build darwin || linux

package modelstore

import (
	"fmt"
	"os"
)

func checkPermissions(path string, info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("private state permissions must exclude group and other users")
	}
	return nil
}
func secureNewDirectory(string) error { return nil }

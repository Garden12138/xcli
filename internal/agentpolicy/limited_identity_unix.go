//go:build darwin || linux

package agentpolicy

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
)

func limitedDirectoryIdentity(path string) (string, error) {
	var st unix.Stat_t
	if unix.Lstat(path, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return "", errors.New("limited_directory_identity_unavailable")
	}
	return fmt.Sprintf("%x:%x", st.Dev, st.Ino), nil
}

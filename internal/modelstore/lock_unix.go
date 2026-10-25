//go:build darwin || linux

package modelstore

import (
	"errors"
	"os"
	"syscall"
)

// The inode remains on disk; closing releases the OS lock, including after crashes.
func Lock(path string, wait bool) (*os.File, error) {
	if err := checkPrivate(path, false); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	flags := syscall.LOCK_EX
	if !wait {
		flags |= syscall.LOCK_NB
	}
	if err := syscall.Flock(int(f.Fd()), flags); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return f, nil
}

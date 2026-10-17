//go:build darwin || linux

package task

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

var ErrOwned = errors.New("task already has an active owner")

// Lock files must never be unlinked: contenders must lock the same inode.
func (s *Store) Lock(id, name string, wait bool) (*os.File, error) {
	if !validID(id) || (name != "owner.lock" && name != "control.lock" && name != "attempt.lock") {
		return nil, errors.New("invalid task lock")
	}
	if err := os.MkdirAll(s.Dir(id), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(s.Dir(id), name), os.O_CREATE|os.O_RDWR, 0o600)
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
			return nil, ErrOwned
		}
		return nil, err
	}
	return f, nil
}
func (s *Store) Active(id, name string) (bool, error) {
	f, err := s.Lock(id, name, false)
	if errors.Is(err, ErrOwned) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	f.Close()
	return false, nil
}

//go:build darwin || linux

package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func (s *State) lock() (func(), error) {
	if s.locked {
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("another MCP operation holds the state lock: %w", err)
	}
	s.locked = true
	return func() { s.locked = false; _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
}

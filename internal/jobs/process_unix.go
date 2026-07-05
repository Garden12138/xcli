//go:build darwin || linux
// +build darwin linux

package jobs

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func ConfigureDetached(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func Terminate(pid int, force bool) error {
	if pid <= 0 {
		return errors.New("job has no valid pid")
	}
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	if err := syscall.Kill(-pid, signal); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func lockExclusive(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func lockHeld(path string) (bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false, err
	}
	defer file.Close()
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	return false, nil
}

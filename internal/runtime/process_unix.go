//go:build darwin || linux
// +build darwin linux

package runtime

import (
	"os"
	"os/exec"
	"syscall"
)

func configureProcess(command *exec.Cmd, separate bool) {
	if separate {
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
}

func terminateProcess(command *exec.Cmd, separate bool) {
	if command.Process == nil {
		return
	}
	if separate {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		return
	}
	_ = command.Process.Signal(os.Interrupt)
}

func killProcess(command *exec.Cmd, separate bool) {
	if command.Process == nil {
		return
	}
	if separate {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		return
	}
	_ = command.Process.Kill()
}

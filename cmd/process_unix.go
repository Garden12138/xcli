//go:build darwin || linux

package cmd

import (
	"os"
	"syscall"
)

func closeFileOnExec(f *os.File) { syscall.CloseOnExec(int(f.Fd())) }

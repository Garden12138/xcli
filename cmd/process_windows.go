//go:build windows

package cmd

import (
	"os"
	"syscall"
)

func closeFileOnExec(f *os.File) { syscall.CloseOnExec(syscall.Handle(f.Fd())) }

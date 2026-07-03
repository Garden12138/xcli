package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
)

type ProcessOptions struct {
	Dir             string
	Env             []string
	Stdin           io.Reader
	Stdout          io.Writer
	Stderr          io.Writer
	CaptureStdout   bool
	SeparateProcess bool
}

type ProcessResult struct {
	ExitCode int
	Stdout   []byte
	Canceled bool
	TimedOut bool
}

func RunProcess(ctx context.Context, spec agent.CommandSpec, options ProcessOptions) (ProcessResult, error) {
	command := exec.Command(spec.Command, spec.Args...)
	command.Dir = options.Dir
	command.Env = options.Env
	command.Stdin = options.Stdin
	configureProcess(command, options.SeparateProcess)

	var captured bytes.Buffer
	if options.CaptureStdout {
		if options.Stdout != nil {
			command.Stdout = io.MultiWriter(options.Stdout, &captured)
		} else {
			command.Stdout = &captured
		}
	} else {
		command.Stdout = options.Stdout
	}
	command.Stderr = options.Stderr

	if err := command.Start(); err != nil {
		return ProcessResult{}, err
	}

	waited := make(chan error, 1)
	go func() {
		waited <- command.Wait()
	}()

	select {
	case err := <-waited:
		return ProcessResult{ExitCode: exitCode(err), Stdout: captured.Bytes()}, nil
	case <-ctx.Done():
		terminateProcess(command, options.SeparateProcess)
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-waited:
		case <-timer.C:
			killProcess(command, options.SeparateProcess)
			<-waited
		}
		result := ProcessResult{ExitCode: 130, Stdout: captured.Bytes(), Canceled: true}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.ExitCode = 124
			result.TimedOut = true
		}
		return result, nil
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
		if status, ok := exitErr.Sys().(interface{ Signal() os.Signal }); ok && status.Signal() != nil {
			return 128
		}
	}
	return 1
}

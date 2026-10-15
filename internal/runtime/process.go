package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
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
	RawOutput       io.Writer
}

type ProcessResult struct {
	ExitCode         int
	Stdout           []byte
	Canceled         bool
	TimedOut         bool
	CaptureTruncated bool
	StderrTail       []byte
	IOError          bool
}

func RunProcess(ctx context.Context, spec agent.CommandSpec, options ProcessOptions) (ProcessResult, error) {
	if ctx.Err() != nil {
		return ProcessResult{ExitCode: -1, Canceled: !errors.Is(ctx.Err(), context.DeadlineExceeded), TimedOut: errors.Is(ctx.Err(), context.DeadlineExceeded)}, nil
	}
	command := exec.Command(spec.Command, spec.Args...)
	command.Dir = options.Dir
	command.Env = options.Env
	if command.Env != nil && options.Dir != "" {
		// A configured environment must not carry the caller's stale PWD into
		// a different project or isolated checkout.
		directory, err := filepath.Abs(options.Dir)
		if err != nil {
			return ProcessResult{ExitCode: -1}, err
		}
		directory, err = filepath.EvalSymlinks(directory)
		if err != nil {
			return ProcessResult{ExitCode: -1}, err
		}
		command.Env = append([]string(nil), options.Env...)
		filtered := command.Env[:0]
		for _, value := range command.Env {
			if !strings.HasPrefix(value, "PWD=") {
				filtered = append(filtered, value)
			}
		}
		command.Env = append(filtered, "PWD="+directory)
	}
	command.Stdin = options.Stdin
	configureProcess(command, options.SeparateProcess)

	var captured boundedBuffer
	if options.CaptureStdout {
		if options.Stdout != nil {
			command.Stdout = io.MultiWriter(options.Stdout, &captured)
		} else {
			command.Stdout = &captured
		}
	} else {
		command.Stdout = options.Stdout
	}
	if options.RawOutput != nil {
		if command.Stdout == nil {
			command.Stdout = options.RawOutput
		} else {
			command.Stdout = io.MultiWriter(command.Stdout, options.RawOutput)
		}
	}
	tail := &tailWriter{writer: options.Stderr}
	command.Stderr = tail

	if err := command.Start(); err != nil {
		return ProcessResult{ExitCode: -1}, err
	}

	waited := make(chan error, 1)
	go func() {
		waited <- command.Wait()
	}()

	select {
	case err := <-waited:
		code, ioFailed := waitOutcome(command, err)
		return ProcessResult{ExitCode: code, IOError: ioFailed, Stdout: captured.Bytes(), CaptureTruncated: captured.truncated, StderrTail: tail.tail}, nil
	case <-ctx.Done():
		terminateProcess(command, options.SeparateProcess)
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		var waitErr error
		select {
		case waitErr = <-waited:
		case <-timer.C:
			killProcess(command, options.SeparateProcess)
			waitErr = <-waited
		}
		code, ioFailed := waitOutcome(command, waitErr)
		result := ProcessResult{ExitCode: code, IOError: ioFailed, Stdout: captured.Bytes(), Canceled: true, CaptureTruncated: captured.truncated, StderrTail: tail.tail}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.Canceled = false
			result.TimedOut = true
		}
		return result, nil
	}
}

func waitOutcome(command *exec.Cmd, err error) (int, bool) {
	var exitErr *exec.ExitError
	ioFailed := err != nil && !errors.As(err, &exitErr)
	if command.ProcessState != nil {
		return command.ProcessState.ExitCode(), ioFailed
	}
	return exitCode(err), ioFailed
}

type tailWriter struct {
	writer io.Writer
	tail   []byte
}

func (w *tailWriter) Write(data []byte) (int, error) {
	const limit = 64 << 10
	if len(data) >= limit {
		w.tail = append(w.tail[:0], data[len(data)-limit:]...)
	} else {
		if excess := len(w.tail) + len(data) - limit; excess > 0 {
			copy(w.tail, w.tail[excess:])
			w.tail = w.tail[:len(w.tail)-excess]
		}
		w.tail = append(w.tail, data...)
	}
	if w.writer != nil {
		return w.writer.Write(data)
	}
	return len(data), nil
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
		return -1
	}
	return 1
}

type boundedBuffer struct {
	bytes.Buffer
	truncated bool
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	n := len(data)
	remaining := agent.MaxOutputBytes - b.Len()
	if len(data) > remaining {
		data = data[:remaining]
		b.truncated = true
	}
	_, err := b.Buffer.Write(data)
	return n, err
}

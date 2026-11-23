//go:build unix

package proc

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type Spec struct {
	Path string
	Args []string
	Dir  string
	Env  []string
}
type Options struct {
	Timeout     time.Duration
	OnLine      func([]byte)
	StdoutFile  string
	StderrFile  string
	RedactProxy bool
}
type Outcome struct {
	ExitCode        int
	TimedOut        bool
	Cancelled       bool
	Started         bool
	StartErr        error
	StderrTail      string
	Duration        time.Duration
	ProcessDuration time.Duration
}

type tailWriter struct {
	mu   sync.Mutex
	data []byte
	file *os.File
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = append(w.data, p...)
	if len(w.data) > 64*1024 {
		w.data = append([]byte(nil), w.data[len(w.data)-64*1024:]...)
	}
	if w.file != nil {
		return w.file.Write(p)
	}
	return len(p), nil
}

func logFile(path string) (*os.File, error) {
	if path == "" {
		return nil, nil
	}
	return os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
}

func Run(ctx context.Context, s Spec, o Options) (out Outcome) {
	start := time.Now()
	out.ExitCode = -1
	defer func() { out.Duration = time.Since(start) }()
	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}
	markContext := func() {
		out.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		out.Cancelled = errors.Is(ctx.Err(), context.Canceled)
	}
	if ctx.Err() != nil {
		markContext()
		return out
	}
	stdoutFile, err := logFile(o.StdoutFile)
	if err != nil {
		out.StartErr = err
		return out
	}
	if stdoutFile != nil {
		defer stdoutFile.Close()
	}
	stderrFile, err := logFile(o.StderrFile)
	if err != nil {
		out.StartErr = err
		return out
	}
	if stderrFile != nil {
		defer stderrFile.Close()
	}
	tail := &tailWriter{file: stderrFile}
	var raw io.Writer
	if stdoutFile != nil {
		raw = stdoutFile
	}
	var stderr io.Writer = tail
	onLine := o.OnLine
	var stdoutLog, stderrLog *redactedLog
	if o.RedactProxy {
		env := s.Env
		if env == nil {
			env = os.Environ()
		}
		if replace := proxyRedactor(env); replace != nil {
			if raw != nil {
				stdoutLog = &redactedLog{dst: raw, replace: replace}
				raw = stdoutLog
			}
			stderrLog = &redactedLog{dst: tail, replace: replace}
			stderr = stderrLog
			if onLine != nil {
				onLine = func(line []byte) { o.OnLine([]byte(replace.Replace(string(line)))) }
			}
		}
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		out.StartErr = err
		return out
	}
	defer reader.Close()
	defer writer.Close()
	cmd := exec.CommandContext(ctx, s.Path, s.Args...)
	cmd.Dir, cmd.Env = s.Dir, s.Env
	cmd.Stdout, cmd.Stderr = writer, stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 6 * time.Second
	var killTimer *time.Timer
	var cancelMu sync.Mutex
	var killAt time.Time
	cmd.Cancel = func() error {
		cancelMu.Lock()
		defer cancelMu.Unlock()
		killAt = time.Now().Add(5 * time.Second)
		killTimer = time.AfterFunc(5*time.Second, func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	processStart := time.Now()
	if err := cmd.Start(); err != nil {
		out.StartErr = err
		markContext()
		return out
	}
	out.Started = true
	_ = writer.Close()
	linesDone := make(chan error, 1)
	go func() {
		defer reader.Close()
		err := readLines(reader, raw, onLine, tail)
		if stdoutLog != nil {
			err = errors.Join(err, stdoutLog.flush())
		}
		linesDone <- err
	}()
	waitErr := cmd.Wait()
	out.ProcessDuration = time.Since(processStart)
	if stderrLog != nil {
		_ = stderrLog.flush()
	}
	select {
	case err = <-linesDone:
	case <-time.After(6 * time.Second):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = reader.Close()
		err = <-linesDone
	}
	cancelMu.Lock()
	if killTimer != nil {
		for syscall.Kill(-cmd.Process.Pid, 0) == nil && time.Now().Before(killAt) {
			time.Sleep(20 * time.Millisecond)
		}
		killTimer.Stop()
		if syscall.Kill(-cmd.Process.Pid, 0) == nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}
	cancelMu.Unlock()
	out.ExitCode = cmd.ProcessState.ExitCode()
	if (err != nil || errors.Is(waitErr, exec.ErrWaitDelay)) && out.ExitCode == 0 {
		out.ExitCode = 1
	}
	markContext()
	tail.mu.Lock()
	out.StderrTail = string(tail.data)
	tail.mu.Unlock()
	return out
}

const maxOutputLine = 8 * 1024 * 1024

func readLines(r io.Reader, raw io.Writer, onLine func([]byte), notices io.Writer) error {
	reader := bufio.NewReaderSize(r, 64*1024)
	var pending []byte
	dropped := false
	for {
		chunk, err := reader.ReadSlice('\n')
		if raw != nil {
			if _, writeErr := raw.Write(chunk); writeErr != nil {
				return writeErr
			}
		}
		end := len(chunk) > 0 && chunk[len(chunk)-1] == '\n'
		if end {
			chunk = chunk[:len(chunk)-1]
		}
		if !dropped {
			if len(pending)+len(chunk) > maxOutputLine {
				dropped, pending = true, nil
				_, _ = io.WriteString(notices, "[xcli] stdout 单行超过 8 MB，已跳过该行；请查看原始 stdout 日志。\n")
			} else {
				pending = append(pending, chunk...)
			}
		}
		if end {
			if !dropped && onLine != nil {
				onLine(pending)
			}
			pending, dropped = nil, false
		}
		if errors.Is(err, io.EOF) {
			if !dropped && len(pending) > 0 && onLine != nil {
				onLine(pending)
			}
			return nil
		}
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
			return err
		}
	}
}

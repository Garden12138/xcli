//go:build darwin || linux
// +build darwin linux

package runtime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
)

func TestRunProcessCapturesOutputEnvironmentAndExitCode(t *testing.T) {
	directory := t.TempDir()
	script := writeProcessScript(t, directory, "process",
		"#!/bin/sh\nprintf '%s:%s' \"$XCLI_PROCESS_VALUE\" \"$1\"\nprintf 'stderr' >&2\nexit 7\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result, err := RunProcess(context.Background(), agent.CommandSpec{
		Command: script, Args: []string{"argument"},
	}, ProcessOptions{
		Dir: directory, Env: []string{"XCLI_PROCESS_VALUE=environment"},
		Stdout: &stdout, Stderr: &stderr, CaptureStdout: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 7 || string(result.Stdout) != "environment:argument" ||
		stdout.String() != "environment:argument" || stderr.String() != "stderr" {
		t.Fatalf("unexpected process result: %#v stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}
}

func TestRunProcessReportsTimeoutAndCancellation(t *testing.T) {
	directory := t.TempDir()
	script := writeProcessScript(t, directory, "slow",
		"#!/bin/sh\ntrap 'exit 0' TERM INT\nwhile :; do sleep 1; done\n")

	timeoutContext, timeoutCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer timeoutCancel()
	timedOut, err := RunProcess(timeoutContext, agent.CommandSpec{Command: script}, ProcessOptions{
		Dir: directory, SeparateProcess: true, CaptureStdout: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if timedOut.ExitCode != 124 || !timedOut.Canceled || !timedOut.TimedOut {
		t.Fatalf("unexpected timeout result: %#v", timedOut)
	}

	cancelContext, cancel := context.WithCancel(context.Background())
	cancel()
	canceled, err := RunProcess(cancelContext, agent.CommandSpec{Command: script}, ProcessOptions{
		Dir: directory, SeparateProcess: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if canceled.ExitCode != 130 || !canceled.Canceled || canceled.TimedOut {
		t.Fatalf("unexpected cancellation result: %#v", canceled)
	}
}

func TestRunProcessReturnsStartError(t *testing.T) {
	_, err := RunProcess(context.Background(), agent.CommandSpec{
		Command: filepath.Join(t.TempDir(), "missing"),
	}, ProcessOptions{})
	if err == nil {
		t.Fatal("expected process start error")
	}
}

func writeProcessScript(t *testing.T, directory, name, body string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

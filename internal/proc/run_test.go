//go:build unix

package proc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunLinesInOrder(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	out := Run(context.Background(), Spec{Path: "/bin/sh", Args: []string{"-c", "printf 'a\\nb\\nc'"}}, Options{StdoutFile: filepath.Join(dir, "stdout"), OnLine: func(b []byte) { lines = append(lines, string(b)) }})
	raw, err := os.ReadFile(filepath.Join(dir, "stdout"))
	if err != nil || out.ExitCode != 0 || !reflect.DeepEqual(lines, []string{"a", "b", "c"}) || string(raw) != "a\nb\nc" {
		t.Fatalf("run = %+v, %v, %q, %v", out, lines, raw, err)
	}
}

func TestRunTimeoutKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	out := Run(context.Background(), Spec{Path: "/bin/sh", Args: []string{"-c", `sleep 30 & echo $! > "$1"; wait`, "sh", pidFile}, Dir: dir}, Options{Timeout: 300 * time.Millisecond})
	if !out.TimedOut || out.Cancelled {
		t.Fatalf("timeout = %+v", out)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 1 {
		t.Fatalf("child pid = %q, %v", raw, err)
	}
	deadline := time.Now().Add(time.Second)
	for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		t.Fatalf("grandchild %d still alive", pid)
	}
}

func TestRunCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	start := time.Now()
	out := Run(ctx, Spec{Path: "/bin/sh", Args: []string{"-c", "sleep 30"}}, Options{})
	if !out.Cancelled || out.TimedOut || time.Since(start) >= 6*time.Second {
		t.Fatalf("cancel = %+v, elapsed=%s", out, time.Since(start))
	}
}

func TestRunKillsChildIgnoringTerm(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "stubborn.pid")
	script := `sh -c 'trap "" TERM; echo $$ > "$1"; exec sleep 30' sh "$1" >/dev/null 2>&1 & wait`
	start := time.Now()
	out := Run(context.Background(), Spec{Path: "/bin/sh", Args: []string{"-c", script, "sh", pidFile}}, Options{Timeout: 200 * time.Millisecond})
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 1 {
		t.Fatalf("child pid = %q, %v", raw, err)
	}
	deadline := time.Now().Add(time.Second)
	for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("child %d survived escalation", pid)
	}
	if !out.TimedOut || time.Since(start) < 5*time.Second || time.Since(start) > 7*time.Second {
		t.Fatalf("escalation: %+v, elapsed=%s", out, time.Since(start))
	}
}

func TestRunExitCode(t *testing.T) {
	out := Run(context.Background(), Spec{Path: "/bin/sh", Args: []string{"-c", "exit 7"}}, Options{})
	if out.ExitCode != 7 || out.StartErr != nil {
		t.Fatalf("exit = %+v", out)
	}
}

func TestRunStartErr(t *testing.T) {
	out := Run(context.Background(), Spec{Path: filepath.Join(t.TempDir(), "missing")}, Options{})
	if out.StartErr == nil {
		t.Fatal("missing executable must report start error")
	}
}

func TestStderrTail(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input")
	text := strings.Repeat("a", 40*1024) + strings.Repeat("b", 64*1024)
	if err := os.WriteFile(input, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	out := Run(context.Background(), Spec{Path: "/bin/sh", Args: []string{"-c", `cat "$1" >&2`, "sh", input}}, Options{StderrFile: filepath.Join(dir, "stderr")})
	raw, err := os.ReadFile(filepath.Join(dir, "stderr"))
	if out.StderrTail != strings.Repeat("b", 64*1024) || err != nil || string(raw) != text {
		t.Fatalf("stderr bytes=%d, raw=%d, err=%v", len(out.StderrTail), len(raw), err)
	}
}

func TestLongLineDropped(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input")
	if err := os.WriteFile(input, []byte(strings.Repeat("x", 9*1024*1024)+"\nok\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var lines []string
	out := Run(context.Background(), Spec{Path: "/bin/sh", Args: []string{"-c", `cat "$1"`, "sh", input}}, Options{OnLine: func(b []byte) { lines = append(lines, string(b)) }})
	if !reflect.DeepEqual(lines, []string{"ok"}) || out.ExitCode != 0 || !strings.Contains(out.StderrTail, "8 MB") {
		t.Fatalf("long line: lines=%v out=%+v", lines, out)
	}
}

func TestRunEnvironmentAndEmptyCancel(t *testing.T) {
	var lines []string
	out := Run(context.Background(), Spec{Path: "/bin/sh", Args: []string{"-c", `printf '%s' "$EXAMPLE"`}, Env: []string{"EXAMPLE=hello"}}, Options{OnLine: func(b []byte) { lines = append(lines, string(b)) }})
	if out.ExitCode != 0 || fmt.Sprint(lines) != "[hello]" {
		t.Fatalf("env: %+v %v", out, lines)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if out := Run(ctx, Spec{Path: "/bin/sh", Args: []string{"-c", "exit 0"}}, Options{}); !out.Cancelled {
		t.Fatalf("already cancelled: %+v", out)
	}
}

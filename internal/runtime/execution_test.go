package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
)

func TestExecutionHelper(t *testing.T) {
	if os.Getenv("XCLI_R0_EXEC_HELPER") != "1" {
		return
	}
	if os.Getenv("XCLI_R0_EXEC_AGY_DENIED") == "1" {
		_, _ = os.Stderr.WriteString(`jetski: no output produced — a tool required the "command" permission that headless mode cannot prompt for, so it was auto-denied.` + "\n")
		for i := 0; i < 5000; i++ {
			_, _ = os.Stderr.WriteString("later native diagnostic\n")
		}
		_, _ = os.Stdout.WriteString("{\"event\":\"result\",\"result\":{\"status\":\"SUCCESS\",\"response\":\"\",\"conversation_id\":\"denied-session\"}}\n")
		os.Exit(0)
	}
	if os.Getenv("XCLI_R0_EXEC_SMALL") == "1" {
		_, _ = os.Stdout.WriteString("{\"type\":\"turn.completed\"}\n")
		os.Exit(0)
	}
	_, _ = os.Stdout.WriteString("{\"type\":\"thread.started\",\"thread_id\":\"s\"}\n")
	for i := 0; i < 2000; i++ {
		_, _ = os.Stdout.WriteString("{\"type\":\"unknown\"}\n")
	}
	_, _ = os.Stdout.WriteString("{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1}}\n")
	os.Exit(0)
}

func TestAntigravityPermissionDenialFailsDespiteNativeSuccess(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, stderr := range []io.Writer{nil, io.Discard} {
		var last agent.Event
		outcome, err := Execute(context.Background(), agent.Definition{Name: "antigravity", Config: config.AgentConfig{Adapter: "antigravity"}}, agent.CommandSpec{Command: binary, Args: []string{"-test.run=^TestExecutionHelper$"}}, ProcessOptions{Stderr: stderr, Env: append(os.Environ(), "XCLI_R0_EXEC_HELPER=1", "XCLI_R0_EXEC_AGY_DENIED=1")}, "r", "", 1, func(event agent.Event) error {
			last = event
			return nil
		})
		if err != nil || outcome.Process.ExitCode != 0 || outcome.Result.ExitCode != 0 || outcome.CLIExitCode != 1 || outcome.Result.Status != "failed" || outcome.Result.ParseStatus != "complete" || outcome.Result.ErrorKind != "process_failed" || outcome.Result.SessionID != "denied-session" || last.Type != "failed" {
			t.Fatalf("%+v last=%+v error=%v", outcome, last, err)
		}
	}
}

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, errors.New("recording unavailable") }

func TestRecordingFailurePreservesNativeZeroExit(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	result, err := Execute(context.Background(), agent.Definition{Config: config.AgentConfig{Adapter: "codex"}}, agent.CommandSpec{Command: binary, Args: []string{"-test.run=^TestExecutionHelper$"}}, ProcessOptions{Env: append(os.Environ(), "XCLI_R0_EXEC_HELPER=1", "XCLI_R0_EXEC_SMALL=1"), RawOutput: failedOutput{}}, "run", "", 1, nil)
	if err != nil || result.Process.ExitCode != 0 || result.Result.ExitCode != 0 || result.CLIExitCode != 1 || result.Result.Status != "failed" {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestSlowEventSinkRetainsTerminalAndUsage(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	definition := agent.Definition{Name: "test", Config: config.AgentConfig{Adapter: "codex"}}
	var last agent.Event
	count := 0
	outcome, err := Execute(context.Background(), definition, agent.CommandSpec{Command: binary, Args: []string{"-test.run=^TestExecutionHelper$"}}, ProcessOptions{Env: append(os.Environ(), "XCLI_R0_EXEC_HELPER=1")}, "r", "", 1, func(event agent.Event) error {
		last = event
		count++
		if event.Type == "progress" {
			time.Sleep(time.Millisecond)
		}
		return nil
	})
	if err != nil || outcome.CLIExitCode != 0 || last.Type != "completed" || outcome.Result.Usage == nil || count < 2 {
		t.Fatalf("%+v %+v %v", outcome, last, err)
	}
}

func TestSinkErrorCancelsAndReturnsError(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("sink unavailable")
	_, err = Execute(context.Background(), agent.Definition{Config: config.AgentConfig{Adapter: "generic"}}, agent.CommandSpec{Command: binary, Args: []string{"-test.run=^TestExecutionHelper$"}}, ProcessOptions{Env: append(os.Environ(), "XCLI_R0_EXEC_HELPER=1")}, "r", "", 1, func(agent.Event) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("%v", err)
	}
}

func TestCanceledContextNeverStartsNativeProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	process, err := RunProcess(ctx, agent.CommandSpec{Command: "does-not-exist"}, ProcessOptions{})
	if err != nil || !process.Canceled || process.ExitCode != -1 {
		t.Fatalf("%+v %v", process, err)
	}
}

func TestConfiguredEnvironmentUsesActualPWD(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("requires POSIX shell")
	}
	directory := t.TempDir()
	process, err := RunProcess(context.Background(), agent.CommandSpec{Command: shell, Args: []string{"-c", "printf '%s' \"$PWD\""}}, ProcessOptions{Dir: directory, Env: []string{"PWD=/stale"}, CaptureStdout: true, Stderr: io.Discard})
	// The shell may canonicalize macOS /var to /private/var.
	actual, resolveErr := os.Stat(string(process.Stdout))
	expected, expectedErr := os.Stat(directory)
	if err != nil || resolveErr != nil || expectedErr != nil || !os.SameFile(actual, expected) {
		t.Fatalf("%s %v", process.Stdout, err)
	}
}

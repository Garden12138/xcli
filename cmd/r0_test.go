//go:build darwin || linux

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/jobs"
	"github.com/Garden12138/xcli/internal/runstore"
	"github.com/Garden12138/xcli/internal/workflow"
)

func r0Config(t *testing.T, body string) (string, *runstore.Store) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-codex")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.DefaultAgent = "test"
	cfg.Agents["test"] = config.AgentConfig{Adapter: "codex", Command: script}
	path := filepath.Join(dir, "config.yaml")
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(dir, "data"))
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	return path, store
}

func TestBrokenStreamFailsEveryExecutionEntry(t *testing.T) {
	for _, mode := range []string{"text", "json", "events", "resume", "workflow", "background"} {
		t.Run(mode, func(t *testing.T) {
			configPath, store := r0Config(t, "printf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"s\"}' 'invalid secret-token' '{\"type\":\"turn.completed\"}'\n")
			root := newRootCommand()
			var out, errOut bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errOut)
			args := []string{"--config", configPath, "run", "goal"}
			switch mode {
			case "json":
				args = append(args, "--json")
			case "events":
				args = append(args, "--events")
			case "resume":
				args = []string{"--config", configPath, "resume", "s", "goal", "--agent", "test", "--json"}
			case "workflow":
				file := filepath.Join(filepath.Dir(configPath), "workflow.yaml")
				if err := os.WriteFile(file, []byte("version: 1\nname: broken\nsteps:\n  - id: step\n    agent: test\n    prompt: goal\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				args = []string{"--config", configPath, "workflow", "run", file, "--json"}
			case "background":
				installDetachedTestHelper(t)
				args = append(args, "--detach", "--json")
			}
			root.SetArgs(args)
			err := root.Execute()
			if mode == "background" {
				if err != nil {
					t.Fatal(err)
				}
				var view jobs.View
				if err := json.Unmarshal(out.Bytes(), &view); err != nil {
					t.Fatal(err)
				}
				record := waitForJob(t, store, view.ID, "failed")
				if record.ExitCode != 0 || record.ParseStatus != "parse_error" {
					t.Fatalf("%+v", record)
				}
				wait := newRootCommand()
				wait.SetOut(&bytes.Buffer{})
				wait.SetErr(&bytes.Buffer{})
				wait.SetArgs([]string{"jobs", "wait", view.ID, "--json"})
				err = wait.Execute()
			}
			var exit *ExitError
			if !errors.As(err, &exit) || exit.Code != 1 {
				t.Fatalf("%s: expected exit 1, got %v (%s)", mode, err, errOut.String())
			}
			records, err := store.List()
			if err != nil || len(records) != 1 {
				t.Fatalf("%+v %v", records, err)
			}
			record := records[0]
			if record.Status != "failed" || record.ParseStatus != "parse_error" || (mode != "workflow" && record.ExitCode != 0) || strings.Contains(record.Error, "secret-token") {
				t.Fatalf("%+v", record)
			}
			if mode == "json" || mode == "resume" {
				var result agent.RunResult
				if err := json.Unmarshal(out.Bytes(), &result); err != nil {
					t.Fatalf("not a single object: %s", out.String())
				}
				if result.RunID != record.ID || result.ExitCode != 0 || result.Status != "failed" {
					t.Fatalf("%+v", result)
				}
			}
			if mode == "events" {
				decoder := json.NewDecoder(&out)
				var last agent.Event
				for decoder.More() {
					if err := decoder.Decode(&last); err != nil {
						t.Fatal(err)
					}
				}
				if last.Type != "failed" {
					t.Fatalf("%+v", last)
				}
			}
		})
	}
}

type activityWriter struct {
	mu     sync.Mutex
	seen   chan struct{}
	closed bool
}

func (w *activityWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed && bytes.Contains(data, []byte(`"type":"progress"`)) {
		close(w.seen)
		w.closed = true
	}
	return len(data), nil
}

func TestEventsAreVisibleBeforeProcessFinishes(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "emitted")
	setCommandEnvironment(t, "XCLI_R0_SIGNAL_PATH", marker)
	path, _ := r0Config(t, "printf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"s\"}'\nprintf ready > \"$XCLI_R0_SIGNAL_PATH\"\nsleep 30\n")
	root := newRootCommand()
	writer := &activityWriter{seen: make(chan struct{})}
	root.SetOut(writer)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", path, "run", "goal", "--events"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root.SetContext(ctx)
	done := make(chan error, 1)
	go func() { done <- root.Execute() }()
	// Measure delivery after the fixture actually emits; startup and race
	// instrumentation are not part of the 500 ms event delivery budget.
	startup := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(startup) {
			cancel()
			<-done
			t.Fatal("fixture did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case <-writer.seen:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no progress within 500 ms")
	}
	cancel()
	select {
	case err := <-done:
		var exit *ExitError
		if !errors.As(err, &exit) || exit.Code != 130 {
			t.Fatalf("%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel deadlocked")
	}
}

func TestWorkflowWorktreeProtectsOriginalAndRecordsCwd(t *testing.T) {
	for _, detached := range []bool{false, true} {
		t.Run(map[bool]string{false: "foreground", true: "background"}[detached], func(t *testing.T) {
			if detached {
				installDetachedTestHelper(t)
			}
			dir := t.TempDir()
			repo := filepath.Join(dir, "repo")
			if err := os.MkdirAll(filepath.Join(repo, "sub"), 0o700); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(dir, "agent")
			if err := os.WriteFile(script, []byte("#!/bin/sh\nif [ \"$1\" = --version ]; then echo 1.0; exit 0; fi\nprintf '%s' \"$PWD\" > actual.cwd\nprintf '%s' \"$PWD\"\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repo, "sub", "tracked"), []byte("baseline"), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"init"}, {"add", "."}, {"-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "baseline"}} {
				if output, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
					t.Fatalf("%s %v", output, err)
				}
			}
			configPath := filepath.Join(dir, "config.yaml")
			cfg := config.Defaults()
			cfg.Agents["fake"] = config.AgentConfig{Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"}, Output: "text"}
			if err := config.Save(configPath, cfg); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, "workflow.yaml")
			definition := workflow.Workflow{Version: 1, Name: "isolation", Cwd: repo, Steps: []workflow.Step{{ID: "edit", Agent: "fake", Prompt: "edit", Cwd: filepath.Join(repo, "sub")}}}
			data, _ := json.Marshal(definition) // JSON is valid YAML, including absolute cwd.
			if err := os.WriteFile(file, data, 0o600); err != nil {
				t.Fatal(err)
			}
			setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(dir, "data"))
			root := newRootCommand()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&bytes.Buffer{})
			args := []string{"--config", configPath, "workflow", "run", file, "--worktree", "--json"}
			if detached {
				args = append(args, "--detach")
			}
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			store, _ := newStore()
			var record runstore.Record
			if detached {
				var view jobs.View
				if err := json.Unmarshal(out.Bytes(), &view); err != nil {
					t.Fatal(err)
				}
				record = waitForJob(t, store, view.ID, "success")
			} else {
				records, err := store.List()
				if err != nil || len(records) != 1 {
					t.Fatalf("%v %+v", err, records)
				}
				record = records[0]
			}
			if record.Status != "success" || record.WorktreePath == "" || len(record.Steps) != 1 || record.Steps[0].Cwd != filepath.Join(record.WorktreePath, "sub") {
				t.Fatalf("%+v", record)
			}
			if _, err := os.Stat(filepath.Join(repo, "sub", "actual.cwd")); !os.IsNotExist(err) {
				t.Fatal("wrote to original checkout")
			}
			marker, err := os.ReadFile(filepath.Join(record.WorktreePath, "sub", "actual.cwd"))
			if err != nil || string(marker) != record.Steps[0].Cwd {
				t.Fatalf("%s %v", marker, err)
			}
		})
	}
}

func TestDoctorVersionFailureIsUnhealthy(t *testing.T) {
	path, _ := r0Config(t, "exit 9\n")
	for _, jsonOutput := range []bool{false, true} {
		root := newRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&bytes.Buffer{})
		args := []string{"--config", path, "doctor", "test"}
		if jsonOutput {
			args = append(args, "--json")
		}
		root.SetArgs(args)
		var exit *ExitError
		if err := root.Execute(); !errors.As(err, &exit) || exit.Code != 1 {
			t.Fatalf("%v", err)
		}
		if !strings.Contains(out.String(), "version check exited with code 9") {
			t.Fatalf("%s", out.String())
		}
	}
}

func TestAntigravityDeniedPermissionFailsExecutionEntries(t *testing.T) {
	for _, mode := range []string{"text", "json", "events", "resume", "workflow", "background"} {
		t.Run(mode, func(t *testing.T) {
			path, store := r0Config(t, `printf '%s\n' 'jetski: no output produced — a tool required the "command" permission that headless mode cannot prompt for, so it was auto-denied.' >&2
printf '%s\n' '{"event":"result","result":{"status":"SUCCESS","response":"","conversation_id":"denied-session"}}'
`)
			cfg, _, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			definition := cfg.Agents["test"]
			definition.Adapter = "antigravity"
			cfg.Agents["test"] = definition
			if err := config.Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			var out, errOut bytes.Buffer
			root := newRootCommand()
			root.SetOut(&out)
			root.SetErr(&errOut)
			args := []string{"--config", path, "run", "goal"}
			switch mode {
			case "json", "events":
				args = append(args, "--"+mode)
			case "resume":
				args = []string{"--config", path, "resume", "denied-session", "goal", "--agent", "test", "--json"}
			case "workflow":
				file := filepath.Join(filepath.Dir(path), "workflow.yaml")
				if err := os.WriteFile(file, []byte("version: 1\nname: denied\nsteps:\n  - id: step\n    agent: test\n    prompt: goal\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				args = []string{"--config", path, "workflow", "run", file, "--json"}
			case "background":
				installDetachedTestHelper(t)
				args = append(args, "--detach", "--json")
			}
			root.SetArgs(args)
			err = root.Execute()
			if mode == "background" {
				if err != nil {
					t.Fatal(err)
				}
				var view jobs.View
				if err := json.Unmarshal(out.Bytes(), &view); err != nil {
					t.Fatal(err)
				}
				waitForJob(t, store, view.ID, "failed")
				wait := newRootCommand()
				wait.SetOut(&bytes.Buffer{})
				wait.SetErr(&bytes.Buffer{})
				wait.SetArgs([]string{"jobs", "wait", view.ID, "--json"})
				err = wait.Execute()
			}
			var exit *ExitError
			if !errors.As(err, &exit) || exit.Code != 1 {
				t.Fatalf("expected exit 1, got %v: %s", err, errOut.String())
			}
			records, err := store.List()
			if err != nil || len(records) != 1 {
				t.Fatalf("%+v %v", records, err)
			}
			record := records[0]
			if record.Status != "failed" || record.CLIExitCode() != 1 {
				t.Fatalf("%+v", record)
			}
			if mode != "workflow" && (record.ExitCode != 0 || record.ParseStatus != "complete" || record.ErrorKind != "process_failed" || record.SessionID != "denied-session") {
				t.Fatalf("%+v", record)
			}
			if mode == "events" && (!strings.Contains(out.String(), `"type":"failed"`) || strings.Contains(out.String(), `"type":"completed"`)) {
				t.Fatalf("unexpected terminal event: %s", out.String())
			}
		})
	}
}

func TestInteractiveUseAndResumeStreamExplicitRecording(t *testing.T) {
	for _, resume := range []bool{false, true} {
		t.Run(map[bool]string{false: "use", true: "resume"}[resume], func(t *testing.T) {
			path, store := r0Config(t, "printf 'interactive transcript'\n")
			cfg, _, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Recording.Output = true
			if err := config.Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			args := []string{"--config", path, "use", "test"}
			if resume {
				source := runstore.Record{ID: "source", Kind: "run", Agent: "test", SessionID: "session", Cwd: filepath.Dir(path), Status: "success", StartedAt: time.Now().Add(-time.Second)}
				if err := store.Save(source); err != nil {
					t.Fatal(err)
				}
				args = []string{"--config", path, "resume", "source"}
			}
			root := newRootCommand()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&bytes.Buffer{})
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			records, err := store.List()
			if err != nil || len(records) == 0 {
				t.Fatalf("%v", err)
			}
			record := records[0]
			raw, err := os.ReadFile(record.OutputFile)
			if err != nil || string(raw) != out.String() || string(raw) != "interactive transcript" || record.ParseStatus != "not_applicable" || record.Status != "success" {
				t.Fatalf("%+v %q %v", record, raw, err)
			}
		})
	}
}

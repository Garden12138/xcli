package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/task"
)

func taskCLI(t *testing.T, path string, args ...string) (task.Task, error) {
	t.Helper()
	root := newRootCommand()
	var out, diagnostics bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&diagnostics)
	if path != "" {
		args = append([]string{"--config", path}, args...)
	}
	root.SetArgs(args)
	err := root.Execute()
	var result task.Task
	if out.Len() > 0 {
		if e := json.Unmarshal(out.Bytes(), &result); e != nil {
			t.Fatalf("%s\n%v\n%s", out.String(), err, diagnostics.String())
		}
	}
	return result, err
}
func waitTask(t *testing.T, id string, predicate func(task.Task) bool) task.Task {
	t.Helper()
	store, err := task.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	var result task.Task
	for time.Now().Before(deadline) {
		result, err = store.Inspect(id)
		if err != nil {
			t.Fatal(err)
		}
		if predicate(result) {
			return result
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("Task did not reach expected state: %+v", result)
	return result
}
func waitGate(t *testing.T, gate string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(gate + ".pid")
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil {
				return pid
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("worker did not reach gate")
	return 0
}
func assertChildExited(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("native child %d survived loss of owner", pid)
}
func TestTaskDetachedStopAndNativeResume(t *testing.T) {
	path := taskCLIConfig(t)
	gate := filepath.Join(t.TempDir(), "gate")
	t.Setenv("XCLI_R2_GATE", gate)
	first, err := taskCLI(t, path, "task", "write a file", "--agent", "test", "--detach", "--json")
	if err != nil {
		t.Fatal(err)
	}
	nativePID := waitGate(t, gate)
	stopped, err := taskCLI(t, "", "task", "stop", first.ID, "--json")
	if err != nil || stopped.Status != "paused" {
		t.Fatalf("%+v %v", stopped, err)
	}
	assertChildExited(t, nativePID)
	again, err := taskCLI(t, "", "task", "stop", first.ID, "--json")
	if err != nil || again.Calls != stopped.Calls {
		t.Fatalf("%+v %v", again, err)
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	launched, err := taskCLI(t, path, "task", "continue", first.ID, "--detach", "--json")
	if err != nil || launched.Status != "running" || launched.ExitCode != 0 {
		t.Fatalf("detached continuation returned stale failure: %+v %v", launched, err)
	}
	result := waitTask(t, first.ID, func(t task.Task) bool { return !t.OwnerActive && t.Status == "completed" })
	if result.Status != "completed" || result.Calls != 4 || len(result.Handoffs) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	argv, _ := os.ReadFile(gate + ".argv")
	if !strings.Contains(string(argv), "exec resume") || !strings.Contains(string(argv), "sandbox_mode=") {
		t.Fatalf("native session was not resumed with role policy: %s", argv)
	}
	store, _ := task.NewStore()
	events, err := store.Events(first.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range events {
		if e.Sequence != uint64(i+1) {
			t.Fatal(e)
		}
	}
}
func TestTaskOwnerKillStopsNativeAndRequiresReconciliation(t *testing.T) {
	path := taskCLIConfig(t)
	gate := filepath.Join(t.TempDir(), "gate")
	t.Setenv("XCLI_R2_GATE", gate)
	first, err := taskCLI(t, path, "task", "write a file", "--agent", "test", "--detach", "--json")
	if err != nil {
		t.Fatal(err)
	}
	nativePID := waitGate(t, gate)
	if err := syscall.Kill(first.PID, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	assertChildExited(t, nativePID)
	lost := waitTask(t, first.ID, func(t task.Task) bool { return !t.OwnerActive && t.Status == "needs_reconcile" })
	if lost.Calls != 2 {
		t.Fatal(lost.Calls)
	}
	result, err := taskCLI(t, path, "task", "continue", first.ID, "--json")
	if err == nil || result.Status != "needs_reconcile" || result.Calls != 2 {
		t.Fatalf("%+v %v", result, err)
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err = taskCLI(t, path, "task", "continue", first.ID, "--reconcile", "--fresh-session", "--json")
	if err != nil || result.Status != "completed" || result.Mode != "single" || result.Agent != "test" {
		t.Fatalf("%+v %v", result, err)
	}
	if len(result.Attempts) < 4 || result.Attempts[1].RunID != lost.Attempts[1].RunID {
		t.Fatal("lost interrupted attempt")
	}
}
func TestTaskLiveUpdateAndCrossTerminalEvents(t *testing.T) {
	path := taskCLIConfig(t)
	gate := filepath.Join(t.TempDir(), "gate")
	t.Setenv("XCLI_R2_GATE", gate)
	first, err := taskCLI(t, path, "task", "write a file", "--agent", "test", "--detach", "--json")
	if err != nil {
		t.Fatal(err)
	}
	waitGate(t, gate)
	queued, err := taskCLI(t, path, "task", "continue", first.ID, "Keep the original deliverable; check it carefully.", "--detach", "--json")
	if err != nil || len(queued.Pending) != 1 || queued.PID != first.PID {
		t.Fatalf("%+v %v", queued, err)
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result := waitTask(t, first.ID, func(t task.Task) bool { return !t.OwnerActive && t.Status == "completed" })
	if result.Calls != 5 || len(result.Updates) != 1 || len(result.Plans) != 2 {
		t.Fatalf("%+v", result)
	}
	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"task", "status", first.ID, "--events", "--after", "1", "--follow"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&out)
	previous := uint64(1)
	for decoder.More() {
		var e task.Event
		if err := decoder.Decode(&e); err != nil {
			t.Fatal(err)
		}
		if e.Sequence != previous+1 {
			t.Fatal(e)
		}
		previous = e.Sequence
	}
	if previous != result.Sequence {
		t.Fatal("missing replayed events")
	}
}

func TestTaskStopImmediatelyAfterDetach(t *testing.T) {
	path := taskCLIConfig(t)
	gate := filepath.Join(t.TempDir(), "gate")
	t.Setenv("XCLI_R2_GATE", gate)
	first, err := taskCLI(t, path, "task", "write a file", "--agent", "test", "--detach", "--json")
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := taskCLI(t, "", "task", "stop", first.ID, "--timeout", "8s", "--json")
	if err != nil || stopped.Status != "paused" {
		t.Fatalf("startup stop was lost: %+v %v", stopped, err)
	}
}

func TestTaskMissingNativeSessionRestoresMaterials(t *testing.T) {
	path := taskCLIConfig(t)
	gate := filepath.Join(t.TempDir(), "gate")
	t.Setenv("XCLI_R2_GATE", gate)
	script := filepath.Join(filepath.Dir(path), "agent")
	body, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	rejection := "#!/bin/sh\ncase \"$*\" in *'exec resume'*) printf 'Error: thread/resume failed: no rollout found for thread id fixture-session (code -32600)\\n' >&2; exit 1;; esac\n"
	if err := os.WriteFile(script, []byte(strings.Replace(string(body), "#!/bin/sh\n", rejection, 1)), 0o700); err != nil {
		t.Fatal(err)
	}
	first, err := taskCLI(t, path, "task", "write a file", "--agent", "test", "--detach", "--json")
	if err != nil {
		t.Fatal(err)
	}
	waitGate(t, gate)
	stopped, err := taskCLI(t, "", "task", "stop", first.ID, "--json")
	if err != nil || stopped.Status != "paused" {
		t.Fatalf("%+v %v", stopped, err)
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := taskCLI(t, path, "task", "continue", first.ID, "--json")
	if err != nil || result.Status != "completed" || result.Calls != 5 {
		t.Fatalf("%+v %v", result, err)
	}
	store, _ := task.NewStore()
	events, err := store.Events(first.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.Type == "session_unavailable" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing fallback evidence")
	}
	receiptData, err := os.ReadFile(filepath.Join(store.Dir(first.ID), result.Attempts[2].RunID+".receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt task.Receipt
	if err := json.Unmarshal(receiptData, &receipt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(receipt.Diagnostics, "no rollout found") {
		t.Fatal("supervisor lost stderr diagnostics")
	}
}

func TestTaskStopIdleFailureIsIdempotent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	store, err := task.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	original := task.Task{Version: 2, ID: "task-idle-failure", Status: "failed", ExitCode: 1, Reason: "retained original failure"}
	if err := store.Save(&original); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		stopped, err := taskCLI(t, "", "task", "stop", original.ID, "--json")
		if err != nil || stopped.Status != "failed" || stopped.ExitCode != 1 {
			t.Fatalf("stop lost original failure or returned an action failure: %+v %v", stopped, err)
		}
	}
}

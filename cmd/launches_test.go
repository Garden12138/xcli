//go:build darwin || linux

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/launch"
)

func TestR41RunResumeProbeBackgroundAndBudget(t *testing.T) {
	for _, mode := range []string{"foreground", "background"} {
		t.Run(mode, func(t *testing.T) {
			configPath, store := r0Config(t, `if [ "$1" = --version ]; then echo fake-cli-1; exit 0; fi
printf '%s\n' '{"type":"thread.started","thread_id":"r41-session"}' '{"type":"item.completed","item":{"type":"agent_message","text":"OK"}}' '{"type":"turn.completed"}'
`)
			if mode == "background" {
				installDetachedTestHelper(t)
			}
			ref := launch.Ref{Root: filepath.Join(t.TempDir(), "ledger"), Window: "authorized"}
			n := 0
			if e := launch.Create(ref, launch.Window{ID: ref.Window, Limit: 3, MaxConcurrent: 1, External: launch.External{Agent: "fake", Count: &n, Source: "fixture", Confirmed: true, Scope: "included"}}); e != nil {
				t.Fatal(e)
			}
			invoke := func(window string, args ...string) (string, error) {
				c := newRootCommand()
				var out bytes.Buffer
				c.SetOut(&out)
				c.SetErr(&bytes.Buffer{})
				c.SetArgs(append([]string{"--config", configPath, "--launch-store", ref.Root, "--launch-window", window}, args...))
				e := c.Execute()
				return out.String(), e
			}
			out, e := invoke(ref.Window, "run", "goal", "--json", "--launch-request", "request-1")
			if mode == "background" {
				out, e = invoke(ref.Window, "run", "goal", "--json", "--detach", "--launch-request", "request-2")
			}
			if e != nil {
				t.Fatal(e, out)
			}
			var result agent.RunResult
			if mode == "background" {
				var job struct {
					ID string `json:"id"`
				}
				if e = json.Unmarshal([]byte(out), &job); e != nil {
					t.Fatal(e, out)
				}
				record := waitForJob(t, store, job.ID, "success")
				if record.LaunchID == "" || !launch.SameRef(ref, record.LaunchRef) {
					t.Fatal(record)
				}
			} else {
				if e = json.Unmarshal([]byte(out), &result); e != nil {
					t.Fatal(e)
				}
				if result.LaunchID == "" {
					t.Fatal(result)
				}
			}
			// Replayed request never creates a native process, including in a worker.
			if mode == "foreground" {
				if _, e = invoke(ref.Window, "run", "goal", "--json", "--launch-request", "request-1"); e == nil {
					t.Fatal("duplicate dispatched")
				}
			}
			before, _ := launch.Stats(ref)
			if _, e = invoke(ref.Window, "launches", "probe", "test", "--kind", "version"); e != nil {
				t.Fatal(e)
			}
			after, _ := launch.Stats(ref)
			if after.Charged != before.Charged+1 {
				t.Fatal(after)
			}
			if mode == "foreground" {
				if _, e = invoke(ref.Window, "resume", result.RunID, "again", "--json"); e != nil {
					t.Fatal(e)
				}
			}
			// All three available starts are now consumed; resume must be blocked.
			if _, e = invoke(ref.Window, "resume", "r41-session", "again", "--agent", "test", "--json"); e == nil {
				t.Fatal("resume bypassed budget")
			}
			if _, e = invoke("missing", "run", "goal", "--json"); e == nil {
				t.Fatal("missing baseline launched")
			}
			state, e := launch.Snapshot(ref)
			if e != nil {
				t.Fatal(e)
			}
			if len(state.Records) != 3 {
				t.Fatal(state)
			}
			for _, r := range state.Records {
				if r.PID == nil || r.StartState != "started" || r.ExitCode == nil || r.ModelRequests != nil || r.Cost != nil || (r.Kind != "version" && r.RunID == "") {
					t.Fatal(r)
				}
			}
		})
	}
}

func TestR41LegacyHistoryPreservedAndCrossWindowResumeBlocked(t *testing.T) {
	configPath, store := r0Config(t, "exit 0\n")
	// Pre-ledger Run JSON lacks new fields; ledger queries must leave bytes untouched.
	legacy := []byte(`{"id":"run-old","kind":"run","agent":"test","session_id":"old-session","status":"success","exit_code":0,"cwd":"/tmp"}`)
	// Use the existing store API first to find its record path through data layout.
	path := filepath.Join(os.Getenv("XDG_DATA_HOME"), "xcli", "runs", "run-old.json")
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, legacy, 0600); e != nil {
		t.Fatal(e)
	}
	ref := launch.Ref{Root: filepath.Join(t.TempDir(), "ledger"), Window: "old-r4"}
	c := newRootCommand()
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"--config", configPath, "--launch-store", ref.Root, "launches", "record-legacy", "old-r4", "--source", "ae0b4be"})
	if e := c.Execute(); e != nil {
		t.Fatal(e)
	}
	stats, e := launch.Stats(ref)
	if e != nil || stats.Remaining != nil || stats.LegacyHistory != "UNKNOWN" {
		t.Fatal(stats, e)
	}
	if _, e = store.Load("run-old"); e != nil {
		t.Fatal(e)
	}
	now, _ := os.ReadFile(path)
	if !bytes.Equal(now, legacy) {
		t.Fatal("legacy bytes changed")
	}
	// A tracked session cannot move to another window in the same store.
	n := 0
	for _, id := range []string{"a", "b"} {
		if e = launch.Create(ref, launch.Window{ID: id, Limit: 2, MaxConcurrent: 1, External: launch.External{Agent: "fake", Count: &n, Source: "fixture", Confirmed: true, Scope: "included"}}); e != nil {
			t.Fatal(e)
		}
	}
	ref.Window = "a"
	ticket, e := launch.Reserve(ref, launch.Metadata{Agent: "test", Adapter: "codex", SessionID: "tracked"}, "fake", "fp")
	if e != nil {
		t.Fatal(e)
	}
	if e = ticket.NotStarted("fixture-no-spawn"); e != nil {
		t.Fatal(e)
	}
	c = newRootCommand()
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetContext(context.Background())
	c.SetArgs([]string{"--config", configPath, "--launch-store", ref.Root, "--launch-window", "b", "resume", "tracked", "goal", "--agent", "test", "--json"})
	if e = c.Execute(); e == nil || !strings.Contains(e.Error(), "window_mismatch") {
		t.Fatal(e)
	}
}

func TestR41TaskDiscoveryStagesAndPinnedContinue(t *testing.T) {
	path := taskCLIConfig(t)
	ref := launch.Ref{Root: filepath.Join(t.TempDir(), "ledger"), Window: "task-window"}
	n := 0
	for _, id := range []string{ref.Window, "another-window"} {
		if e := launch.Create(ref, launch.Window{ID: id, Limit: 8, MaxConcurrent: 1, External: launch.External{Agent: "fake", Count: &n, Source: "fixture", Confirmed: true, Scope: "included"}}); e != nil {
			t.Fatal(e)
		}
	}
	first, e := taskCLI(t, path, "--launch-store", ref.Root, "--launch-window", ref.Window, "task", "write a file", "--agent", "test", "--json")
	if e != nil || first.Status != "completed" {
		t.Fatal(first, e)
	}
	o, e := launch.Stats(ref)
	if e != nil || o.Charged != 8 || *o.Remaining != 0 {
		t.Fatal(o, e)
	}
	s, e := launch.Snapshot(ref)
	if e != nil {
		t.Fatal(e)
	}
	phases := map[string]bool{}
	for _, v := range s.Records {
		if v.TaskID != first.ID {
			t.Fatal(v)
		}
		if v.Kind != "version" && v.Kind != "help" {
			if v.RunID == "" || v.ParentTask != first.ID || v.Attempt != 1 || v.Version != "fixture-1" {
				t.Fatal(v)
			}
			phases[v.Phase] = true
		}
	}
	if len(phases) != 3 {
		t.Fatal(phases)
	}
	if _, e = taskCLI(t, path, "--launch-store", ref.Root, "--launch-window", "another-window", "task", "continue", first.ID, "--json"); e == nil || !strings.Contains(e.Error(), "window_mismatch") {
		t.Fatal(e)
	}
}

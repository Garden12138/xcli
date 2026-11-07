package agentservice

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/agentstore"
	"github.com/Garden12138/xcli/internal/nativeadapter"
)

func crashProfile() agentpolicy.Profile {
	return agentpolicy.Profile{AgentID: "fake", AdapterID: "fake-v1", PolicyID: "test-only", Revision: "1", Location: "remote", Operations: []string{"text_task"}}
}

type crashDriver struct{ Root, Mode string }

func (d crashDriver) StartTurn(context.Context, nativeadapter.BoundRequest, func(nativeadapter.Observation) error) nativeadapter.TurnResult {
	f, _ := os.OpenFile(filepath.Join(d.Root, "fake-starts"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString("start\n")
	_ = f.Sync()
	_ = f.Close()
	if d.Mode == "spawn" {
		os.Exit(99)
	}
	zero := 0
	return nativeadapter.TurnResult{Text: "crash fixture result", NativeID: "crash-native", SessionObserved: true, Terminal: "success", Protocol: "complete", ExitCode: &zero, LocalStopConfirmed: true}
}
func TestCrashWriterHelper(t *testing.T) {
	if os.Getenv("XCLI_NAG_CRASH_HELPER") != "1" {
		return
	}
	root, mode := os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]
	st, e := agentstore.Open(root)
	if e != nil {
		t.Fatal(e)
	}
	cs := &agentstore.Clients{Root: st.Root}
	c := agentstore.Client{ID: "worklog", Scopes: agentstore.Scopes, Agents: []string{"fake"}, Policies: []string{"test-only"}, Operations: []string{"text_task"}, AllowRemote: true, DailyDispatches: 6, DailyProcessStarts: 6, DailyExecutionSeconds: 3600}
	if _, e = cs.Issue(c, false); e != nil {
		t.Fatal(e)
	}
	cfg := agentpolicy.DefaultConfig()
	cfg.DailyDispatches = 6
	cfg.DailyProcessStarts = 6
	cfg.DailyExecutionSeconds = 3600
	s, e := New(st, cs, cfg, []agentpolicy.Profile{crashProfile()})
	if e != nil {
		t.Fatal(e)
	}
	ses := createSession(t, s, c, s.Profiles[0])
	if _, _, e = s.CreateTask(c, taskRequest(ses), "crash-task-key"); e != nil {
		t.Fatal(e)
	}
	if mode == "accepted" {
		os.Exit(99)
	}
	count := 0
	st.BeforeSync = func() error {
		count++
		if (mode == "dispatch-write" && count == 1) || (mode == "terminal-write" && count == 2) {
			os.Exit(99)
		}
		return nil
	}
	if _, e = s.ExecuteNext(context.Background(), crashDriver{st.Root, mode}); e != nil {
		t.Fatal(e)
	}
	if e = st.Checkpoint(); e != nil {
		t.Fatal(e)
	}
	os.Exit(99)
}
func TestActualProcessCrashNeverRedispatches(t *testing.T) {
	for _, mode := range []string{"accepted", "dispatch-write", "spawn", "terminal-write", "checkpoint"} {
		t.Run(mode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "state")
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCrashWriterHelper$", "--", root, mode)
			command.Env = []string{"PATH=/usr/bin:/bin", "XCLI_NAG_CRASH_HELPER=1", "GORACE=atexit_sleep_ms=0"}
			b, e := command.CombinedOutput()
			exit, ok := e.(*exec.ExitError)
			if !ok || exit.ExitCode() != 99 {
				t.Fatalf("crash helper %v: %s", e, b)
			}
			st, e := agentstore.Open(root)
			if e != nil {
				t.Fatal(e)
			}
			defer st.Close()
			cfg := agentpolicy.DefaultConfig()
			cfg.DailyDispatches = 6
			cfg.DailyProcessStarts = 6
			cfg.DailyExecutionSeconds = 3600
			s, e := New(st, &agentstore.Clients{Root: st.Root}, cfg, []agentpolicy.Profile{crashProfile()})
			if e != nil {
				t.Fatal(e)
			}
			d := &fakeDriver{}
			if _, e = s.ExecuteNext(context.Background(), d); e != nil {
				t.Fatal(e)
			}
			if d.calls != 0 {
				t.Fatal("crash replay started a second turn")
			}
			_ = st.View(func(state agentstore.State) error {
				for _, r := range state.Tasks {
					if !api.Terminal(r.Task.Status) {
						t.Fatal("recovery left executable task", r.Task.Status)
					}
					if mode == "spawn" && r.Task.Status != "needs_reconcile" {
						t.Fatal(r.Task)
					}
					if mode == "checkpoint" && r.Task.Status != "succeeded" {
						t.Fatal(r.Task)
					}
				}
				return nil
			})
			starts, _ := os.ReadFile(filepath.Join(st.Root, "fake-starts"))
			if mode == "spawn" || mode == "terminal-write" || mode == "checkpoint" {
				if string(starts) != "start\n" {
					t.Fatal(fmt.Sprintf("unexpected fake process starts %q", starts))
				}
			} else if len(starts) != 0 {
				t.Fatal("started before dispatch", string(starts))
			}
		})
	}
}

package task

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/workflow"
)

func TestCheckpointContinueFourPaths(t *testing.T) {
	for _, project := range []bool{false, true} {
		for _, single := range []bool{false, true} {
			t.Run(map[bool]string{false: "work", true: "project"}[project]+map[bool]string{false: "/auto", true: "/single"}[single], func(t *testing.T) {
				r, o, _ := fixture(t, "", project, single)
				ctx, cancel := context.WithCancel(context.Background())
				r.Sink = func(e Event) error {
					if e.Type == "handoff" {
						cancel()
					}
					return nil
				}
				first, err := r.Run(ctx, o)
				if err == nil || first.Status != "canceled" || first.Checkpoint.Stage != "accept" {
					t.Fatalf("%+v %v", first, err)
				}
				calls := first.Calls
				time.Sleep(150 * time.Millisecond)
				r.Sink = nil
				next, err := r.Continue(context.Background(), first.ID, Update{})
				if err != nil || next.Status != "completed" || next.Calls != calls+1 || next.ID != first.ID || next.Mode != first.Mode || next.Agent != first.Agent || len(next.Handoffs) != 1 {
					t.Fatalf("%+v %v", next, err)
				}
				if next.Elapsed-first.Elapsed > 5*time.Second {
					t.Fatal("pause consumed execution budget")
				}
				events, err := r.Store.Events(next.ID, 0)
				if err != nil {
					t.Fatal(err)
				}
				for i, e := range events {
					if e.Sequence != uint64(i+1) {
						t.Fatal("nonmonotonic continuation", e)
					}
				}
				again, err := r.Continue(context.Background(), next.ID, Update{})
				if err != nil || again.Calls != next.Calls {
					t.Fatal("completed Task replayed", err)
				}
			})
		}
	}
}

func TestWaitingTaskAcceptsVerification(t *testing.T) {
	r, o, root := fixture(t, "", true, true)
	data, _ := os.ReadFile(filepath.Join(root, "plan.json"))
	var p Plan
	_ = json.Unmarshal(data, &p)
	p.Actions = append(p.Actions[:1], p.Actions[2:]...)
	data, _ = json.Marshal(p)
	_ = os.WriteFile(filepath.Join(root, "plan.json"), data, 0o600)
	first, err := r.Run(context.Background(), o)
	if err != nil || first.Status != "waiting_user" {
		t.Fatalf("%+v %v", first, err)
	}
	next, err := r.Continue(context.Background(), first.ID, Update{Verify: [][]string{{"go", "test", "-count=1", "./..."}}})
	if err != nil || next.Status != "completed" || next.Calls != 4 || len(next.Handoffs) != 1 {
		t.Fatalf("%+v %v", next, err)
	}
}

func TestCumulativeBudgetAndQueuedInput(t *testing.T) {
	t.Run("raise_call_limit", func(t *testing.T) {
		r, o, _ := fixture(t, "", false, true)
		o.MaxCalls = 2
		first, err := r.Run(context.Background(), o)
		if err == nil || first.Calls != 2 {
			t.Fatalf("%+v %v", first, err)
		}
		next, err := r.Continue(context.Background(), first.ID, Update{MaxCalls: 3})
		if err != nil || next.Status != "completed" || next.Calls != 3 || len(next.Handoffs) != 1 {
			t.Fatalf("%+v %v", next, err)
		}
	})
	t.Run("live_input_at_checkpoint", func(t *testing.T) {
		r, o, _ := fixture(t, "", false, true)
		queued := false
		r.Sink = func(e Event) error {
			if e.Role == "worker" && e.Type == "started" && !queued {
				queued = true
				_, err := r.Store.Queue(e.TaskID, Update{Message: "Keep the original deliverable and verify it."}, false)
				return err
			}
			return nil
		}
		result, err := r.Run(context.Background(), o)
		if err != nil || result.Status != "completed" || len(result.Plans) != 2 || len(result.Updates) != 1 || !strings.Contains(result.Goal, "User update:") {
			t.Fatalf("%+v %v", result, err)
		}
	})
}

func TestPerCallTimeoutSurvivesContinueAndKeepsTotalBudget(t *testing.T) {
	r, o, _ := fixture(t, "", false, true)
	o.MaxCalls = 2
	o.CallTimeout = 8 * time.Minute
	var limits []string
	r.Execute = func(ctx context.Context, id, cwd, boundary string, step workflow.Step, output bool, sink func(agent.Event) error) workflow.StepResult {
		limits = append(limits, step.Timeout)
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > o.Timeout {
			t.Fatal("per-call timeout removed the cumulative deadline")
		}
		wr := workflow.Runner{Config: r.Config, Registry: r.Registry, Store: r.Runs, Events: sink}
		return wr.ExecuteAttempt(ctx, id, cwd, boundary, step, output)
	}
	first, err := r.Run(context.Background(), o)
	if err == nil || first.Calls != 2 || first.Options.CallTimeout != 8*time.Minute {
		t.Fatalf("first Task: %+v %v", first, err)
	}
	next, err := r.Continue(context.Background(), first.ID, Update{MaxCalls: 3, CallTimeout: 10 * time.Minute})
	if err != nil || next.Status != "completed" || next.ID != first.ID || next.Calls != 3 || next.Options.CallTimeout != 10*time.Minute {
		t.Fatalf("continued Task: %+v %v", next, err)
	}
	if strings.Join(limits, ",") != "8m0s,8m0s,10m0s" {
		t.Fatalf("Agent call limits: %v", limits)
	}
}

func TestLegacyCallTimeoutAndInvalidQueuedReduction(t *testing.T) {
	r, o, _ := fixture(t, "", false, true)
	first, err := r.Run(context.Background(), o)
	if err != nil || first.Options.CallTimeout != DefaultCallTimeout {
		t.Fatal("missing compatible default", err)
	}
	first.Options.CallTimeout = 0 // Task saved by a previous binary.
	if err := r.Store.Save(&first); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Store.Queue(first.ID, Update{CallTimeout: 8 * time.Minute}, false); err != nil {
		t.Fatal(err)
	}
	for _, value := range []time.Duration{-time.Second, 5 * time.Minute} {
		if _, err := r.Store.Queue(first.ID, Update{CallTimeout: value}, false); err == nil {
			t.Fatal("accepted reduced pending call timeout", value)
		}
	}
	control, err := r.Store.Control(first.ID)
	if err != nil || len(control.Updates) != 1 {
		t.Fatal("invalid update poisoned the control queue", err)
	}
	next, err := r.Continue(context.Background(), first.ID, Update{})
	if err != nil || next.Options.CallTimeout != 8*time.Minute || next.Calls != first.Calls {
		t.Fatalf("legacy continuation: %+v %v", next, err)
	}
}

func TestJournalAndLockRecovery(t *testing.T) {
	r, o, _ := fixture(t, "", false, true)
	first, err := r.Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("atomic_snapshot_lag_and_partial_tail", func(t *testing.T) {
		dir := r.Store.Dir(first.ID)
		if err := os.WriteFile(filepath.Join(dir, "task.json"), []byte("broken"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"checkpoints.jsonl", "events.jsonl"} {
			f, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = f.WriteString("{partial")
			_ = f.Close()
		}
		next, err := r.Continue(context.Background(), first.ID, Update{})
		if err != nil || next.Calls != first.Calls {
			t.Fatalf("%+v %v", next, err)
		}
		if _, err := r.Store.Events(first.ID, 0); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("single_owner_and_pending_input", func(t *testing.T) {
		lease, err := r.Store.Lock(first.ID, "owner.lock", false)
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Close()
		_, err = r.Store.Lock(first.ID, "owner.lock", false)
		if !errors.Is(err, ErrOwned) {
			t.Fatal(err)
		}
		result, err := r.Continue(context.Background(), first.ID, Update{Message: "queued while owned"})
		if err != nil || len(result.Pending) != 1 {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("complete_corrupt_line_rejected", func(t *testing.T) {
		path := filepath.Join(r.Store.Dir(first.ID), "events.jsonl")
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.WriteString("broken\n")
		_ = f.Close()
		if _, err := r.Store.Events(first.ID, 0); err == nil {
			t.Fatal("silently accepted corrupt journal")
		}
	})
}

func TestInterruptedAttemptReconciliationAndReceipts(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncertain", true: "confirmed_receipt"}[confirmed], func(t *testing.T) {
			r, o, _ := fixture(t, "", false, true)
			ctx, cancel := context.WithCancel(context.Background())
			// Interrupt after a durable worker handoff, then reconstruct the previous
			// in-flight checkpoint to simulate owner loss before committing that node.
			r.Sink = func(e Event) error {
				if e.Type == "handoff" {
					cancel()
				}
				return nil
			}
			result, err := r.Run(ctx, o)
			if err == nil {
				t.Fatal("expected interruption")
			}
			result.Status = "running"
			result.Checkpoint.Stage = "worker"
			result.Checkpoint.InFlight = result.Attempts[1].RunID
			result.Handoffs = nil
			if !confirmed {
				if err := os.Remove(filepath.Join(r.Store.Dir(result.ID), result.Checkpoint.InFlight+".receipt.json")); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.Store.Save(&result); err != nil {
				t.Fatal(err)
			}
			r.Sink = nil
			next, err := r.Continue(context.Background(), result.ID, Update{})
			if confirmed {
				if err != nil || next.Status != "completed" || next.Calls != 3 || len(next.Handoffs) != 1 {
					t.Fatalf("%+v %v", next, err)
				}
			} else {
				if err == nil || next.Status != "needs_reconcile" || next.Calls != result.Calls {
					t.Fatalf("%+v %v", next, err)
				}
				next, err = r.Continue(context.Background(), result.ID, Update{Reconcile: true, FreshSession: true})
				if err != nil || next.Status != "completed" {
					t.Fatalf("%+v %v", next, err)
				}
			}
		})
	}
}

func TestChangedMaterialsAndBindings(t *testing.T) {
	r, o, _ := fixture(t, "", false, true)
	first, err := r.Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first.Output, "result.txt"), []byte("externally changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	next, err := r.Continue(context.Background(), first.ID, Update{})
	if err == nil || next.Status != "needs_reconcile" {
		t.Fatalf("%+v %v", next, err)
	}
	next, err = r.Continue(context.Background(), first.ID, Update{Reconcile: true, FreshSession: true})
	if err != nil || next.Status != "completed" || len(next.Plans) != 2 {
		t.Fatalf("%+v %v", next, err)
	}
	cfg := r.Config.Agents["first"]
	cfg.Command = "changed-login-entry"
	r.Config.Agents["first"] = cfg
	r.Registry = agent.NewRegistry(r.Config)
	if _, err := r.Continue(context.Background(), first.ID, Update{Message: "more work"}); err == nil {
		t.Fatal("silently changed executable/login entry")
	}
}

func TestSessionFailureClassification(t *testing.T) {
	for _, text := range []string{"authentication failed", "quota exceeded", "request timed out", "invalid session response", "TLS connection failed"} {
		if sessionUnavailable(workflow.StepResult{Status: "failed", Error: text}) {
			t.Fatal("unsafe fallback", text)
		}
	}
	if !sessionUnavailable(workflow.StepResult{Status: "failed", Error: "session not found"}) {
		t.Fatal("missing session not recognized")
	}
}

func TestInvalidQueuedBudgetDoesNotPoisonTask(t *testing.T) {
	r, o, _ := fixture(t, "", false, true)
	first, err := r.Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Store.Queue(first.ID, Update{MaxCalls: 1}, false); err == nil {
		t.Fatal("accepted reduced cumulative budget")
	}
	c, err := r.Store.Control(first.ID)
	if err != nil || len(c.Updates) != 0 {
		t.Fatalf("invalid update was persisted: %+v %v", c, err)
	}
}
func TestCorruptControlFailsVisibly(t *testing.T) {
	r, o, _ := fixture(t, "cancel", false, true)
	r.Sink = func(e Event) error {
		if e.Type == "started" {
			return r.Store.Write(e.TaskID, "control.json", []byte("broken"))
		}
		return nil
	}
	result, err := r.Run(context.Background(), o)
	if err == nil || result.Status != "failed" || result.ExitCode != 1 || !strings.Contains(result.Reason, "invalid character") {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestSimulatedRestartAtTrustedCheckpoint(t *testing.T) {
	for _, single := range []bool{false, true} {
		t.Run(map[bool]string{false: "auto", true: "single"}[single], func(t *testing.T) {
			r, o, _ := fixture(t, "", false, single)
			ctx, cancel := context.WithCancel(context.Background())
			r.Sink = func(e Event) error {
				if e.Type == "handoff" {
					cancel()
				}
				return nil
			}
			first, err := r.Run(ctx, o)
			if err == nil {
				t.Fatal("expected interruption")
			}
			// Reboot leaves files and a stale PID but releases all OS leases.
			first.Status = "running"
			first.PID = os.Getpid()
			first.OwnerActive = true
			if err := r.Store.Save(&first); err != nil {
				t.Fatal(err)
			}
			view, err := r.Store.Inspect(first.ID)
			if err != nil || view.Status != "paused" || view.OwnerActive {
				t.Fatalf("%+v %v", view, err)
			}
			r.Sink = nil
			result, err := r.Continue(context.Background(), first.ID, Update{})
			if err != nil || result.Status != "completed" || result.Calls != first.Calls+1 || result.Mode != first.Mode {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

func TestR1TaskRequiresExplicitMaterialRecovery(t *testing.T) {
	r, o, _ := fixture(t, "", false, true)
	first, err := r.Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	first.Version = 1
	first.Checkpoint = nil
	first.Options = Options{}
	first.Bindings = nil
	first.Elapsed = 0
	if err := os.Remove(filepath.Join(r.Store.Dir(first.ID), "checkpoints.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := r.Store.Save(&first); err != nil {
		t.Fatal(err)
	}
	loaded, err := r.Store.Load(first.ID)
	if err != nil || loaded.Version != 1 {
		t.Fatalf("%+v %v", loaded, err)
	}
	if _, err := r.Continue(context.Background(), first.ID, Update{}); err == nil {
		t.Fatal("legacy execution position was guessed")
	}
	result, err := r.Continue(context.Background(), first.ID, Update{Reconcile: true, FreshSession: true})
	if err != nil || result.Status != "completed" || result.ID != first.ID || result.Version != 2 || result.Calls != 6 || len(result.Handoffs) != 2 {
		t.Fatalf("%+v %v", result, err)
	}
}

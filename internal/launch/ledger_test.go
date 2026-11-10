package launch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T, limit, parallel int) Ref {
	t.Helper()
	n := 0
	r := Ref{Root: filepath.Join(t.TempDir(), "ledger"), Window: "test"}
	if e := Create(r, Window{ID: r.Window, Limit: limit, MaxConcurrent: parallel, External: External{Agent: "fake-controller", Count: &n, Source: "explicit fake fixture", Confirmed: true, Scope: "included"}}); e != nil {
		t.Fatal(e)
	}
	return r
}
func fake(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	c := exec.Command(exe, "-test.run=^TestNativeHelper$")
	c.Env = append(os.Environ(), "XCLI_LEDGER_NATIVE_HELPER="+mode)
	return c
}
func TestNativeHelper(t *testing.T) {
	switch os.Getenv("XCLI_LEDGER_NATIVE_HELPER") {
	case "exit":
		os.Exit(0)
	case "fail":
		os.Exit(7)
	case "wait":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
}
func stats(t *testing.T, r Ref) Summary {
	t.Helper()
	o, e := Stats(r)
	if e != nil {
		t.Fatal(e)
	}
	return o
}

func TestHundredConcurrentTasksCannotOverdraw(t *testing.T) {
	r := fixture(t, 7, 100)
	ctx := WithRef(context.Background(), r)
	var passed atomic.Int64
	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := fake(t, "exit")
			ticket, e := Start(ctx, c, Metadata{Agent: "fake", Adapter: "codex", Kind: "run", TaskID: fmt.Sprintf("task-%d", i), RunID: fmt.Sprintf("run-%d", i)})
			if errors.Is(e, ErrBudget) {
				return
			}
			if e != nil {
				errs <- e
				return
			}
			if e = c.Wait(); e != nil {
				errs <- e
			}
			if e = ticket.Finish(0, false, false, true); e != nil {
				errs <- e
			}
			passed.Add(1)
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	if passed.Load() != 7 {
		t.Fatal("actual starts", passed.Load())
	}
	o := stats(t, r)
	if o.Charged != 7 || o.ConfirmedStarts != 7 || *o.Remaining != 0 || o.Active != 0 {
		t.Fatal(o)
	}
}
func TestConcurrentSlotsAndIdempotency(t *testing.T) {
	r := fixture(t, 100, 3)
	var wg sync.WaitGroup
	tickets := make(chan *Ticket, 100)
	var unexpected atomic.Int64
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticket, e := Reserve(r, Metadata{Agent: "fake", Kind: "run"}, "fake", "fp")
			if errors.Is(e, ErrConcurrent) {
				return
			}
			if e != nil {
				unexpected.Add(1)
				return
			}
			tickets <- ticket
		}()
	}
	wg.Wait()
	close(tickets)
	if len(tickets) != 3 || unexpected.Load() != 0 || stats(t, r).Active != 3 {
		t.Fatal("concurrency limit not atomic")
	}
	for ticket := range tickets {
		if e := ticket.NotStarted("fixture_proved_no_spawn"); e != nil {
			t.Fatal(e)
		}
	}
	if stats(t, r).Released != 3 {
		t.Fatal("known non-start was not released")
	}
	meta := Metadata{Agent: "fake", Kind: "run", RequestKey: "same-operation"}
	ticket, e := Reserve(r, meta, "fake", "same-fingerprint")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Reserve(r, meta, "fake", "same-fingerprint"); !errors.Is(e, ErrDuplicate) {
		t.Fatal("duplicate dispatch", e)
	}
	if _, e = Reserve(r, meta, "fake", "changed"); e == nil || !strings.Contains(e.Error(), "conflict") {
		t.Fatal("idempotency conflict", e)
	}
	_ = ticket.NotStarted("no_spawn")
	if _, e = Reserve(r, meta, "fake", "same-fingerprint"); !errors.Is(e, ErrDuplicate) {
		t.Fatal("replayed failed request", e)
	}
}
func TestImmediateExitFailureRetryResumeAndNoRefund(t *testing.T) {
	r := fixture(t, 3, 1)
	ctx := WithRef(context.Background(), r)
	for i, kind := range []string{"run", "resume", "retry"} {
		c := fake(t, "fail")
		ticket, e := Start(ctx, c, Metadata{Agent: "fake", Adapter: "codex", Kind: kind, SessionID: "same-session", RunID: fmt.Sprint(i), Attempt: i + 1, Retry: i > 0})
		if e != nil {
			t.Fatal(e)
		}
		_ = c.Wait()
		if e = ticket.Finish(7, false, false, true); e != nil {
			t.Fatal(e)
		}
		if e = ticket.NotStarted("false claim"); e == nil {
			t.Fatal("started process refunded")
		}
	}
	c := fake(t, "exit")
	if _, e := Start(ctx, c, Metadata{Agent: "fake", Adapter: "codex", Kind: "resume", SessionID: "same-session"}); !errors.Is(e, ErrBudget) {
		t.Fatal("resume bypassed budget", e)
	}
	s, e := Snapshot(r)
	if e != nil {
		t.Fatal(e)
	}
	ids := map[string]bool{}
	for _, v := range s.Records {
		ids[v.ID] = true
		if v.Consumed != 1 || v.ExitCode == nil || *v.ExitCode != 7 || v.PID == nil || v.StartedAt == nil || v.EndedAt == nil {
			t.Fatal(v)
		}
	}
	if len(ids) != 3 || stats(t, r).Charged != 3 {
		t.Fatal("retry identity/consumption")
	}
	n := 0
	other := r
	other.Window = "other"
	if e = Create(other, Window{ID: other.Window, Limit: 10, MaxConcurrent: 1, External: External{Agent: "fixture", Source: "owner", Count: &n, Scope: "included", Confirmed: true}}); e != nil {
		t.Fatal(e)
	}
	if _, e = Start(WithRef(context.Background(), other), fake(t, "exit"), Metadata{Agent: "fake", Adapter: "codex", Kind: "resume", SessionID: "same-session"}); e == nil {
		t.Fatal("session silently moved windows")
	}
}
func TestOnlyProvedNonStartCanRelease(t *testing.T) {
	r := fixture(t, 1, 1)
	ctx := WithRef(context.Background(), r)
	c := exec.Command(filepath.Join(t.TempDir(), "missing"))
	if _, e := Start(ctx, c, Metadata{Agent: "fake", Kind: "run"}); e == nil {
		t.Fatal("missing executable started")
	}
	o := stats(t, r)
	if o.Charged != 0 || o.Released != 1 || *o.Remaining != 1 {
		t.Fatal(o)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := Start(cancelCtx, fake(t, "exit"), Metadata{Agent: "fake", Kind: "run"}); e == nil {
		t.Fatal("canceled start accepted")
	}
	if stats(t, r).Charged != 0 {
		t.Fatal("pre-start cancellation charged")
	}
	c = fake(t, "exit")
	ticket, e := Start(ctx, c, Metadata{Agent: "fake", Kind: "run"})
	if e != nil {
		t.Fatal(e)
	}
	_ = c.Wait()
	// Simulate losing wait evidence: a start never earns a refund.
	if e = ticket.Finish(-1, false, false, false); e != nil {
		t.Fatal(e)
	}
	if _, e = Reserve(r, Metadata{Agent: "fake"}, "fake", "fp"); !errors.Is(e, ErrBudget) {
		t.Fatal(e)
	}
	if e = ConfirmStopped(r, ticket.ID, "fixture stop proof", true); e != nil {
		t.Fatal(e)
	}
	if stats(t, r).Charged != 1 || stats(t, r).Active != 0 {
		t.Fatal("stop confirmation refunded budget")
	}
}
func TestProbeTimeoutAndCancellationAreCharged(t *testing.T) {
	r := fixture(t, 2, 1)
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(WithRef(context.Background(), r), 80*time.Millisecond)
		c := fake(t, "wait")
		c.Cancel = nil
		// Use CommandContext to get a cancellation-aware probe, as agent.Probe does.
		c = exec.CommandContext(ctx, c.Path, c.Args[1:]...)
		c.Env = append(os.Environ(), "XCLI_LEDGER_NATIVE_HELPER=wait")
		if canceled {
			go func() { time.Sleep(20 * time.Millisecond); cancel() }()
		}
		id, e := Run(ctx, c, Metadata{Agent: "fake", Kind: "help"})
		cancel()
		if e == nil || id == "" {
			t.Fatal("probe did not fail/capture", e)
		}
		s, _ := Snapshot(r)
		rec := s.Records[id]
		if rec.Consumed != 1 || rec.Canceled != canceled || rec.TimedOut == canceled {
			t.Fatal(rec)
		}
	}
}

func TestCrashHelper(t *testing.T) {
	mode := os.Getenv("XCLI_LEDGER_CRASH")
	if mode == "" {
		return
	}
	r := Ref{Root: os.Getenv("XCLI_LEDGER_ROOT"), Window: "test"}
	if mode == "after-spawn" {
		c := fake(t, "exit")
		_, e := Start(WithRef(context.Background(), r), c, Metadata{Agent: "fake", Kind: "run"})
		if e != nil {
			os.Exit(90)
		}
		os.Exit(41)
	}
	ticket, e := Reserve(r, Metadata{Agent: "fake", Kind: "run"}, "fake", "fp")
	if e != nil {
		os.Exit(90)
	}
	if mode == "intent" {
		if e = ticket.Starting(); e != nil {
			os.Exit(90)
		}
	}
	os.Exit(41)
}
func TestCrashRecoveryBeforeIntentAfterIntentAndAfterSpawn(t *testing.T) {
	for _, mode := range []string{"reserved", "intent", "after-spawn"} {
		t.Run(mode, func(t *testing.T) {
			r := fixture(t, 2, 1)
			exe, _ := os.Executable()
			c := exec.Command(exe, "-test.run=^TestCrashHelper$")
			c.Env = append(os.Environ(), "XCLI_LEDGER_CRASH="+mode, "XCLI_LEDGER_ROOT="+r.Root)
			if e := c.Run(); e == nil {
				t.Fatal("crash helper survived")
			}
			o := stats(t, r)
			if mode == "reserved" {
				if o.Charged != 0 || o.Released != 1 || o.Active != 0 {
					t.Fatal(o)
				}
			} else {
				if o.Charged != 1 || o.Active != 1 {
					t.Fatal(o)
				}
				if _, e := Reserve(r, Metadata{Agent: "fake"}, "fake", "new"); !errors.Is(e, ErrConcurrent) {
					t.Fatal("unknown owner released concurrency", e)
				}
			}
			// Reopening never resets counts or reconstructs unknown history.
			restarted := stats(t, Ref{Root: r.Root, Window: r.Window})
			if restarted.Charged != o.Charged || restarted.LegacyHistory != "UNKNOWN" {
				t.Fatal(restarted)
			}
		})
	}
}
func TestExternalBaselineAndLegacyAreNeverInferredAsZero(t *testing.T) {
	r := Ref{Root: filepath.Join(t.TempDir(), "ledger"), Window: "legacy-r4"}
	if e := Create(r, Window{ID: r.Window, Limit: 6, MaxConcurrent: 1, External: External{Agent: "codex", Scope: "unknown", Source: "R4 report ae0b4be"}}); e != nil {
		t.Fatal(e)
	}
	o := stats(t, r)
	if o.Remaining != nil || o.Status != "UNKNOWN" {
		t.Fatal("legacy budget fabricated", o)
	}
	if _, e := Reserve(r, Metadata{Agent: "fake"}, "fake", "fp"); !errors.Is(e, ErrUnknown) {
		t.Fatal(e)
	}
	for _, w := range []Window{{ID: "no-confirm", Limit: 6, MaxConcurrent: 1, External: External{Agent: "codex", Scope: "included", Source: "snapshot"}}, {ID: "no-source", Limit: 6, MaxConcurrent: 1, External: External{Agent: "codex", Scope: "excluded", Confirmed: true}}} {
		if e := Create(r, w); e != nil {
			t.Fatal(e)
		}
		next := r
		next.Window = w.ID
		if stats(t, next).Remaining != nil {
			t.Fatal("missing baseline accepted")
		}
	}
	n := 2
	w := Window{ID: "fresh", Limit: 6, MaxConcurrent: 1, External: External{Agent: "codex", Count: &n, Scope: "included", Source: "owner receipt", Confirmed: true}}
	if e := Create(r, w); e != nil {
		t.Fatal(e)
	}
	fresh := r
	fresh.Window = "fresh"
	if *stats(t, fresh).Remaining != 4 {
		t.Fatal("external starts not charged")
	}
	if e := Create(r, w); e == nil {
		t.Fatal("window reset")
	}
	// Unknown or malformed existing schema is blocked, never overwritten.
	before, e := os.ReadFile(filepath.Join(r.Root, "ledger.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(r.Root, "ledger.json"), []byte(`{"version":99}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Snapshot(r); e == nil {
		t.Fatal("future schema reinitialized")
	}
	_ = before
}

func TestConcurrentIdempotencyAndCorruptionFailClosed(t *testing.T) {
	r := fixture(t, 10, 10)
	var wg sync.WaitGroup
	var wins atomic.Int64
	var ticket *Ticket
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := Reserve(r, Metadata{Agent: "fake", RequestKey: "one"}, "fake", "fp")
			if e == nil {
				wins.Add(1)
				ticket = v
			} else if !errors.Is(e, ErrDuplicate) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 || stats(t, r).Charged != 1 {
		t.Fatal(wins.Load())
	}
	if e := ticket.NotStarted("fixture"); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(r.Root, "ledger.json")
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	mutated := strings.Replace(string(b), `"limit":10`, `"limit":11`, 1)
	if mutated == string(b) {
		t.Fatal("fixture did not change")
	}
	if e = os.WriteFile(path, []byte(mutated), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Reserve(r, Metadata{Agent: "fake"}, "fake", "new"); e == nil {
		t.Fatal("corrupt ledger launched")
	}
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if _, e = Stats(r); e == nil {
		t.Fatal("missing used ledger was reset")
	}
}

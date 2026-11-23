package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/stats"
	"github.com/Garden12138/xcli/internal/testutil"
)

func checkCalls(t *testing.T, env *Env, want ...stats.Call) {
	t.Helper()
	got := env.Calls()
	if got == nil || len(got) != len(want) {
		t.Fatalf("calls = %+v, want %+v", got, want)
	}
	for i, call := range got {
		if call.DurationMS < 0 {
			t.Fatalf("invalid duration in call %+v", call)
		}
		call.DurationMS = 0
		if call != want[i] {
			t.Errorf("call %d = %+v, want %+v", i, call, want[i])
		}
	}
}

func TestStandaloneReviewCall(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"cursor-agent": {{Stdout: cursorOK(`{"verdict":"pass","summary":"ok","issues":[]}`)}}}, api.Request{})
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindReview}, "cursor", nil)
	if r.Result.Status != api.StatusCompleted {
		t.Fatalf("review: %+v", r)
	}
	checkCalls(t, e, stats.Call{Agent: "cursor", Purpose: "review", Step: "main", Round: 1})
}

func TestCancelledCall(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{SleepMS: 5000}}}, api.Request{})
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	done := make(chan StepOutcome, 1)
	go func() { done <- e.RunStep(ctx, api.Step{ID: "main", Kind: api.KindAsk}, "codex", nil) }()
	for {
		if _, err := os.Stat(filepath.Join(os.Getenv("XCLI_FAKE_STATE"), "codex-0.pid")); err == nil {
			cancel()
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fake codex did not start")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if r := <-done; r.Result.Status != api.StatusCancelled {
		t.Fatalf("cancelled: %+v", r)
	}
	checkCalls(t, e, stats.Call{Agent: "codex", Purpose: "run", Step: "main", Round: 1, ErrorKind: "cancelled"})
}

func TestCallsRequireProcessStart(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{}, api.Request{})
	step := api.Step{ID: "main", Kind: api.KindAsk}
	exe := filepath.Join(filepath.SplitList(e.PathList)[0], "codex")
	if err := os.WriteFile(exe, []byte("invalid executable format"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, remove := range []bool{false, true} {
		if remove {
			if err := os.Remove(exe); err != nil {
				t.Fatal(err)
			}
		}
		if r := e.RunStep(context.Background(), step, "codex", nil); r.Result.Status != api.StatusFailed {
			t.Fatalf("expected start failure: %+v", r)
		}
		checkCalls(t, e)
	}
}

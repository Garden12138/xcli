//go:build unix

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/store"
	"github.com/Garden12138/xcli/internal/testutil"
)

func nativeOK(text string) []string {
	b, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]string{"type": "agent_message", "text": text}})
	return []string{`{"type":"thread.started","thread_id":"parent-session"}`, string(b), `{"type":"turn.completed"}`}
}

func testEngine(t *testing.T, script testutil.FakeScript) *Engine {
	t.Helper()
	home := t.TempDir()
	cfg := config.Default()
	cfg.Path = testutil.InstallFakeCLIs(t, script)
	for name, a := range cfg.Agents {
		a.ArtifactDirs = []string{}
		cfg.Agents[name] = a
	}
	raw, err := config.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(home, "config.yaml"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	e, err := New(home)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func create(t *testing.T, e *Engine, req api.Request) *store.Task {
	t.Helper()
	task, err := e.Create(req)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestEngineRunAsk(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{"codex": {{Stdout: nativeOK("你好")}}})
	task := create(t, e, api.Request{Capability: api.CapAsk, Goal: "hello", Agent: "codex", Metadata: map[string]string{"app": "test"}})
	var events []api.Event
	r := e.Run(context.Background(), task, func(ev api.Event) { events = append(events, ev) })
	loaded, err := e.Store.Load(task.ID())
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != api.StatusCompleted || r.Summary != "你好" || r.TaskID != task.ID() || r.Capability != api.CapAsk || r.Metadata["app"] != "test" || r.StartedAt.IsZero() || r.FinishedAt.Before(r.StartedAt) || loaded.Snapshot().Result == nil {
		t.Fatalf("result: %+v", r)
	}
	if len(events) < 4 || events[0].Type != "task.started" || events[len(events)-1].Type != "task.finished" {
		t.Fatalf("events: %+v", events)
	}
	ready := false
	for i, ev := range events {
		if ev.Seq != i+1 {
			t.Fatal("event sequence")
		}
		ready = ready || ev.Type == "plan.ready"
	}
	if !ready {
		t.Fatal("missing plan.ready")
	}
	persisted, err := store.ReadEvents(task.Dir(), 0)
	if err != nil || len(persisted) != len(events) {
		t.Fatalf("persisted=%v err=%v", persisted, err)
	}
}

func TestEnginePlanOnly(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{})
	task := create(t, e, api.Request{Goal: "总结文档", PlanOnly: true, Isolate: true, Workdir: t.TempDir()})
	r := e.Run(context.Background(), task, nil)
	if r.Status != api.StatusPlanned || r.Plan == nil || len(testutil.FakeCalls(t)) != 0 {
		t.Fatalf("plan only: %+v", r)
	}
	if _, err := os.Stat(task.Path("worktree")); !os.IsNotExist(err) {
		t.Fatal("plan created worktree")
	}
}

var buildOnce sync.Once
var builtSelf string
var buildErr error

func workerSelf(t *testing.T) {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "xcli-engine-test-")
		if err != nil {
			buildErr = err
			return
		}
		builtSelf = filepath.Join(dir, "xcli")
		cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", builtSelf, ".")
		cmd.Dir = "../.."
		if raw, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildFailure{string(raw), err}
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	t.Setenv("XCLI_SELF", builtSelf)
}

type buildFailure struct {
	text string
	err  error
}

func (e *buildFailure) Error() string { return e.text + ": " + e.err.Error() }

func TestEngineDetachAndWait(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{"codex": {{Stdout: nativeOK("background")}}})
	workerSelf(t)
	task := create(t, e, api.Request{Goal: "hello", Capability: api.CapAsk, Agent: "codex"})
	if err := e.Detach(task); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := e.Wait(ctx, task.ID())
	if err != nil || r.Status != api.StatusCompleted || r.Summary != "background" {
		t.Fatalf("wait: %+v err=%v", r, err)
	}
}

func TestEngineCancel(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{"codex": {{Stdout: nativeOK("late"), SleepMS: 10000}}})
	workerSelf(t)
	task := create(t, e, api.Request{Goal: "sleep", Capability: api.CapAsk, Agent: "codex"})
	if err := e.Detach(task); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	workerPID, childPID := 0, 0
	for time.Now().Before(deadline) {
		rec, err := e.Status(task.ID())
		if err != nil {
			t.Fatal(err)
		}
		if rec.Status == api.StatusRunning && len(testutil.FakeCalls(t)) > 0 {
			workerPID = rec.PID
			raw, _ := os.ReadFile(filepath.Join(os.Getenv("XCLI_FAKE_STATE"), "codex-0.pid"))
			childPID, _ = strconv.Atoi(string(raw))
			if childPID > 0 {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("fake CLI did not start")
	}
	t.Cleanup(func() { _ = syscall.Kill(workerPID, syscall.SIGKILL); _ = syscall.Kill(childPID, syscall.SIGKILL) })
	if err := e.Cancel(task.ID()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := e.Wait(ctx, task.ID())
	if err != nil || r.Status != api.StatusCancelled {
		t.Fatalf("cancel: %+v err=%v", r, err)
	}
	if err := syscall.Kill(childPID, 0); err == nil {
		t.Fatal("fake CLI still running")
	}
}

func TestEngineWorkerLost(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{})
	task := create(t, e, api.Request{Goal: "x"})
	if err := task.Update(func(r *store.Record) { r.Status = api.StatusRunning; r.PID = 1 << 30 }); err != nil {
		t.Fatal(err)
	}
	rec, err := e.Status(task.ID())
	if err != nil || rec.Status != api.StatusFailed || rec.Result == nil || rec.Result.Error.Kind != "worker_lost" {
		t.Fatalf("lost: %+v err=%v", rec, err)
	}
}

func TestEngineResume(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{"codex": {{WritePNG: "{{OUT}}/image.png", Stdout: nativeOK(`{"files":[]}`)}}})
	out := t.TempDir()
	parent := create(t, e, api.Request{Goal: "一张猫图", Capability: api.CapImage, Agent: "codex", OutputDir: out})
	r := e.Run(context.Background(), parent, nil)
	if r.Status != api.StatusCompleted || len(r.Artifacts) != 1 {
		t.Fatalf("parent: %+v", r)
	}
	if err := parent.Update(func(rec *store.Record) {
		rec.Steps = append(rec.Steps, api.StepResult{ID: "skipped", Kind: api.KindReview, Agent: "cursor", Status: api.StatusSkipped})
	}); err != nil {
		t.Fatal(err)
	}
	req, err := e.Resume(parent.ID(), "改成蓝色")
	if err != nil {
		t.Fatal(err)
	}
	if req.Capability != api.CapImage || req.Agent != "codex" || req.ParentTask != parent.ID() || req.OutputDir != out || len(req.Images) != 1 || req.Images[0] != r.Artifacts[0].Path || req.Workdir != parent.Path("work") {
		t.Fatalf("resume request: %+v", req)
	}
	child := create(t, e, req)
	r = e.Run(context.Background(), child, nil)
	calls := testutil.FakeCalls(t)
	if r.Status != api.StatusCompleted || len(calls) != 2 || !strings.Contains(strings.Join(calls[1].Args, " "), "resume") || !strings.Contains(strings.Join(calls[1].Args, " "), "parent-session") {
		t.Fatalf("resume: %+v calls=%+v", r, calls)
	}
}

func TestEngineGC(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{})
	old := create(t, e, api.Request{Goal: "old"})
	running := create(t, e, api.Request{Goal: "running"})
	if err := old.Update(func(r *store.Record) {
		r.Status = api.StatusCompleted
		r.Result = &api.Result{Status: api.StatusCompleted, FinishedAt: time.Now().Add(-8 * 24 * time.Hour)}
	}); err != nil {
		t.Fatal(err)
	}
	if err := running.Update(func(r *store.Record) { r.Status = api.StatusRunning; r.CreatedAt = time.Now().Add(-9 * 24 * time.Hour) }); err != nil {
		t.Fatal(err)
	}
	n, err := e.GC(7 * 24 * time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("gc=%d err=%v", n, err)
	}
	if _, err := os.Stat(old.Dir()); !os.IsNotExist(err) {
		t.Fatal("old task retained")
	}
	if _, err := os.Stat(running.Dir()); err != nil {
		t.Fatal(err)
	}
}

func TestEngineExplicitPlanInvalidBeforeCreate(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{})
	_, err := e.Create(api.Request{Goal: "bad", Plan: &api.Plan{Steps: []api.Step{{ID: "a", Kind: api.KindAsk, Agent: "auto", DependsOn: []string{"a"}}}}})
	rows, _ := e.Store.List(0)
	if err == nil || len(rows) != 0 {
		t.Fatalf("invalid plan created a task: %v %+v", err, rows)
	}
}

func TestEngineWaitTimeoutKeepsTask(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{})
	task := create(t, e, api.Request{Goal: "queued"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := e.Wait(ctx, task.ID())
	rec, _ := e.Status(task.ID())
	if !errors.Is(err, context.DeadlineExceeded) || rec.Status != api.StatusQueued {
		t.Fatalf("wait changed task: %+v err=%v", rec, err)
	}
}

func TestEngineTimeout(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{"codex": {{SleepMS: 10000, Stdout: nativeOK("late")}}})
	task := create(t, e, api.Request{Goal: "timeout", Capability: api.CapAsk, Agent: "codex", TimeoutSec: 1})
	r := e.Run(context.Background(), task, nil)
	if r.Status != api.StatusFailed || r.Error == nil || r.Error.Kind != "timeout" {
		t.Fatalf("timeout: %+v", r)
	}
}

func TestEngineApplyAndGCWorktree(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		raw, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s %v", args, raw, err)
		}
		return string(raw)
	}
	git("init", "-q")
	git("add", ".")
	git("-c", "user.name=test", "-c", "user.email=test@localhost", "-c", "commit.gpgsign=false", "commit", "-qm", "init")
	e := testEngine(t, testutil.FakeScript{"codex": {{WriteFiles: map[string]string{"a.txt": "after\n"}, Stdout: nativeOK("changed")}}})
	off := false
	task := create(t, e, api.Request{Goal: "change", Capability: api.CapCode, Agent: "codex", Workdir: repo, Isolate: true, Review: &off})
	r := e.Run(context.Background(), task, nil)
	if r.Status != api.StatusCompleted || r.Changes == nil || r.Changes.Isolation != "worktree" {
		t.Fatalf("worktree: %+v", r)
	}
	if raw, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(raw) != "before\n" {
		t.Fatal("original changed before apply")
	}
	if err := e.Apply(task.ID(), true); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(raw) != "after\n" {
		t.Fatal("patch not applied")
	}
	if _, err := os.Stat(r.Changes.Workdir); !os.IsNotExist(err) {
		t.Fatal("worktree retained")
	}
	task = create(t, e, api.Request{Goal: "change", Capability: api.CapCode, Agent: "codex", Workdir: repo, Isolate: true, Review: &off})
	r = e.Run(context.Background(), task, nil)
	if r.Status != api.StatusCompleted {
		t.Fatalf("second task: %+v", r)
	}
	if err := task.Update(func(rec *store.Record) { rec.Result.FinishedAt = time.Now().Add(-8 * 24 * time.Hour) }); err != nil {
		t.Fatal(err)
	}
	n, err := e.GC(7 * 24 * time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("GC: %d %v", n, err)
	}
	if strings.Contains(git("branch", "--list"), "xcli/"+task.ID()) {
		t.Fatal("GC retained branch")
	}
}

func TestEngineDetachFailure(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{})
	t.Setenv("XCLI_SELF", filepath.Join(t.TempDir(), "missing"))
	task := create(t, e, api.Request{Goal: "x"})
	err := e.Detach(task)
	rec, statusErr := e.Status(task.ID())
	if err == nil || statusErr != nil || rec.Status != api.StatusFailed || rec.Result == nil {
		t.Fatalf("detach failure: %+v err=%v statusErr=%v", rec, err, statusErr)
	}
}

func TestCommandProcessExitCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		step testutil.FakeStep
		code int
		args []string
	}{
		{"completed", testutil.FakeStep{Stdout: nativeOK("ok")}, 0, []string{"ask", "hi", "-a", "codex", "--json"}},
		{"invalid", testutil.FakeStep{}, 2, []string{"ask", "--json"}},
		{"failed", testutil.FakeStep{Exit: 1}, 1, []string{"ask", "hi", "-a", "codex", "--json"}},
		{"needs_attention", testutil.FakeStep{Stdout: nativeOK("check failed")}, 3, []string{"code", "fix", "-a", "codex", "--no-review", "--verify", "false", "--max-fix-rounds", "0", "--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := testEngine(t, testutil.FakeScript{"codex": {tc.step}})
			workerSelf(t)
			args := append([]string{"--home", e.Home}, tc.args...)
			if tc.name == "needs_attention" {
				args = append(args, "-C", t.TempDir())
			}
			cmd := exec.Command(builtSelf, args...)
			var out, stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			err := cmd.Run()
			code := 0
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				code = exit.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			var result api.Result
			if code != tc.code || json.Unmarshal(out.Bytes(), &result) != nil || string(result.Status) != tc.name || stderr.Len() != 0 {
				t.Fatalf("code=%d output=%s stderr=%s err=%v", code, out.String(), stderr.String(), err)
			}
		})
	}
}

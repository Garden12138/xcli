package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/driver"
	"github.com/Garden12138/xcli/internal/health"
	"github.com/Garden12138/xcli/internal/recipe"
	"github.com/Garden12138/xcli/internal/store"
	"github.com/Garden12138/xcli/internal/testutil"
	"github.com/Garden12138/xcli/internal/workspace"
)

func codexOK(text string) []string {
	message, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]string{"type": "agent_message", "text": text}})
	return []string{`{"type":"thread.started","thread_id":"test-session"}`, string(message), `{"type":"turn.completed","usage":{"input_tokens":5,"output_tokens":2}}`}
}

func cursorOK(text string) []string {
	message, _ := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]string{"text": text}}}})
	return []string{`{"type":"system","session_id":"cursor-session"}`, string(message), `{"type":"result","subtype":"success","session_id":"cursor-session"}`}
}

func stepEnv(t *testing.T, script testutil.FakeScript, req api.Request) *Env {
	t.Helper()
	cfg := config.Default()
	cfg.Path = testutil.InstallFakeCLIs(t, script)
	for name, agent := range cfg.Agents {
		agent.ArtifactDirs = nil
		cfg.Agents[name] = agent
	}
	if req.Goal == "" {
		req.Goal = "test goal"
	}
	if req.Permission == "" {
		req.Permission = api.PermWrite
	}
	home := t.TempDir()
	st, err := store.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	task, err := st.Create(req)
	if err != nil {
		t.Fatal(err)
	}
	ws, _, err := workspace.Prepare(task.Dir(), task.ID(), req)
	if err != nil {
		t.Fatal(err)
	}
	return &Env{Config: cfg, Drivers: driver.All(cfg), Health: health.Open(home), Task: task, WS: ws, Req: req, PathList: cfg.Path, LockDir: filepath.Join(home, "locks"), Now: time.Now}
}

func codeRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# demo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=test", "-c", "user.email=test@localhost", "-c", "commit.gpgsign=false", "commit", "-qm", "init"}} {
		if raw, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git: %s, %v", raw, err)
		}
	}
	return dir
}

func TestRunStepAsk(t *testing.T) {
	fixture, _ := filepath.Abs(filepath.Join("..", "driver", "testdata", "codex", "ask_ok.jsonl"))
	e := stepEnv(t, testutil.FakeScript{"codex": {{StdoutFile: fixture}}}, api.Request{})
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindAsk}, "codex", nil)
	if r.Result.Status != api.StatusCompleted || r.Text == "" || r.Result.SessionID == "" || !strings.Contains(strings.Join(testutil.FakeCalls(t)[0].Args, " "), "-s read-only") {
		t.Fatalf("ask: %+v", r)
	}
	for _, name := range []string{"prompt.md", "stdout.jsonl", "stderr.log", "result.json"} {
		if _, err := os.Stat(e.Task.Path("steps", "main-r1", name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunStepAskSchemaRepair(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"cursor-agent": {{Stdout: cursorOK("not JSON")}, {Stdout: cursorOK(`{"answer":"ok"}`)}}}, api.Request{Schema: json.RawMessage(`{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`)})
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindAsk}, "cursor", nil)
	calls := testutil.FakeCalls(t)
	if r.Result.Status != api.StatusCompleted || !json.Valid(r.JSON) || len(calls) != 2 || !strings.Contains(strings.Join(calls[1].Args, " "), "--resume") {
		t.Fatalf("schema repair: %+v, calls=%+v", r, calls)
	}
}

func TestRunStepAskSchemaMismatch(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"cursor-agent": {{Stdout: cursorOK("not JSON")}}}, api.Request{Schema: json.RawMessage(`{"type":"object"}`)})
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindAsk}, "cursor", nil)
	if r.Result.Status != api.StatusFailed || r.Result.Error == nil || r.Result.Error.Kind != "schema_mismatch" || len(testutil.FakeCalls(t)) != 2 {
		t.Fatalf("mismatch: %+v", r)
	}
}

func TestRunStepImage(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK(`{"files":[]}`), WritePNG: "{{OUT}}/main-1.png"}}}, api.Request{})
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindImage}, "codex", nil)
	if r.Result.Status != api.StatusCompleted || len(r.Artifacts) != 1 || r.Artifacts[0].StepID != "main" || r.Artifacts[0].Agent != "codex" {
		t.Fatalf("image: %+v", r)
	}
}

func TestRunStepImageNoArtifact(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK(`{"files":[]}`)}}}, api.Request{})
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindImage}, "codex", nil)
	if r.Result.Error == nil || r.Result.Error.Kind != "no_artifact" {
		t.Fatalf("missing image: %+v", r)
	}
}

func TestRunStepCodeVerifyFixLoop(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK("first"), WriteFiles: map[string]string{"a.txt": "a"}}, {Stdout: codexOK("fixed"), WriteFiles: map[string]string{"b.txt": "b"}}}}, api.Request{Workdir: codeRepo(t), Verify: []string{"test -f b.txt"}})
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindCode}, "codex", nil)
	calls := testutil.FakeCalls(t)
	if r.Result.Status != api.StatusCompleted || r.Result.Rounds != 2 || len(r.Verify) != 2 || r.Verify[0].Passed || !r.Verify[1].Passed || !strings.Contains(strings.Join(calls[1].Args, " "), "resume") {
		t.Fatalf("verify fix: %+v, calls=%+v", r, calls)
	}
	if r.Changes == nil || len(r.Changes.Files) != 2 {
		t.Fatalf("cumulative changes: %+v", r.Changes)
	}
}

func TestRunStepCodeReviewLoop(t *testing.T) {
	reviewer := "cursor"
	e := stepEnv(t, testutil.FakeScript{
		"codex":        {{Stdout: codexOK("implemented"), WriteFiles: map[string]string{"a.txt": "a"}}, {Stdout: codexOK("fixed")}},
		"cursor-agent": {{Stdout: cursorOK(`{"verdict":"changes_requested","summary":"fix","issues":[{"severity":"major","file":null,"description":"fix boundary"}]}`)}, {Stdout: cursorOK(`{"verdict":"pass","summary":"ok","issues":[]}`)}},
	}, api.Request{Workdir: codeRepo(t)})
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindCode, ReviewBy: &reviewer}, "codex", nil)
	if r.Result.Status != api.StatusCompleted || r.Result.Rounds != 2 || r.Review == nil || r.Review.Verdict != "pass" {
		t.Fatalf("review loop: %+v", r)
	}
	coderLog, _ := os.ReadFile(e.Task.Path("steps", "main-r1", "stdout.jsonl"))
	reviewLog, err := os.ReadFile(e.Task.Path("steps", "main-r1", "review", "stdout.jsonl"))
	if err != nil || !strings.Contains(string(coderLog), "thread.started") || !strings.Contains(string(reviewLog), "changes_requested") {
		t.Fatalf("separate review log: coder=%s reviewer=%s err=%v", coderLog, reviewLog, err)
	}
}

func TestRunStepCodeMinorOnlyPasses(t *testing.T) {
	reviewer := "cursor"
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK("done")}}, "cursor-agent": {{Stdout: cursorOK(`{"verdict":"changes_requested","summary":"minor","issues":[{"severity":"minor","file":null,"description":"style"}]}`)}}}, api.Request{Workdir: codeRepo(t)})
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindCode, ReviewBy: &reviewer}, "codex", nil)
	if r.Result.Status != api.StatusCompleted || r.Result.Rounds != 1 || r.Review == nil || len(r.Review.Issues) != 1 {
		t.Fatalf("minor issue: %+v", r)
	}
}

func TestRunStepCodeMaxRounds(t *testing.T) {
	maxFix := 1
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK("done")}}}, api.Request{Workdir: codeRepo(t), Verify: []string{"false"}, MaxFixRounds: &maxFix})
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindCode}, "codex", nil)
	if r.Result.Status != api.StatusNeedsAttention || r.Result.Rounds != 2 {
		t.Fatalf("round limit: %+v", r)
	}
}

func TestRunStepTimeout(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{SleepMS: 5000}}}, api.Request{})
	e.Config.Defaults.StepTimeout = config.Duration(300 * time.Millisecond)
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindAsk}, "codex", nil)
	if r.Result.Error == nil || r.Result.Error.Kind != "timeout" {
		t.Fatalf("timeout: %+v", r)
	}
}

func TestRunStepCodeResumeFailureStartsNewSession(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {
		{Stdout: codexOK("first"), WriteFiles: map[string]string{"a.txt": "a"}},
		{Stderr: "session not found", Exit: 1},
		{Stdout: codexOK("recovered"), WriteFiles: map[string]string{"b.txt": "b"}},
	}}, api.Request{Workdir: codeRepo(t), Verify: []string{"test -f b.txt"}})
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindCode}, "codex", nil)
	calls := testutil.FakeCalls(t)
	if r.Result.Status != api.StatusCompleted || r.Result.Rounds != 2 || len(calls) != 3 || !strings.Contains(strings.Join(calls[1].Args, " "), "resume") || strings.Contains(strings.Join(calls[2].Args, " "), "exec resume") {
		t.Fatalf("resume recovery: %+v, %+v", r, calls)
	}
	if _, err := os.Stat(e.Task.Path("steps", "main-r2", "resume-failed-stderr.log")); err != nil {
		t.Fatal(err)
	}
}

func TestRunStepNativeSchemaRefusalUsesInline(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","additionalProperties":false,"required":["answer"],"properties":{"answer":{"type":"string"}}}`)
	e := stepEnv(t, testutil.FakeScript{"codex": {
		{Stdout: []string{`{"type":"thread.started","thread_id":"test-session"}`, `{"type":"turn.failed","error":{"message":"invalid schema for response_format"}}`}, Exit: 1},
		{Stdout: codexOK(`{"answer":"ok"}`)},
	}}, api.Request{Schema: schema})
	cfg := e.Config.Agents["codex"]
	cfg.MaxConcurrency = 1
	e.Config.Agents["codex"] = cfg
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r := e.RunStep(ctx, api.Step{ID: "main", Kind: api.KindAsk}, "codex", nil)
	calls := testutil.FakeCalls(t)
	if r.Result.Status != api.StatusCompleted || !json.Valid(r.JSON) || len(calls) != 2 || strings.Contains(strings.Join(calls[1].Args, " "), "--output-schema") {
		t.Fatalf("native schema fallback: %+v, %+v", r, calls)
	}
	if _, err := os.Stat(e.Task.Path("steps", "main-r1", "schema-failed-stdout.jsonl")); err != nil {
		t.Fatal(err)
	}
}

func TestRunStepLaxSchemaUsesPrompt(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK(`{"answer":"ok"}`)}}}, api.Request{Schema: json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"}}}`)})
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindAsk}, "codex", nil)
	calls := testutil.FakeCalls(t)
	if r.Result.Status != api.StatusCompleted || len(calls) != 1 || strings.Contains(strings.Join(calls[0].Args, " "), "--output-schema") || !strings.Contains(calls[0].Args[len(calls[0].Args)-1], string(e.Req.Schema)) {
		t.Fatalf("lax schema: %+v, %+v", r, calls)
	}
}

func TestRunStepConcurrentSinkOrdered(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK("ok"), SleepMS: 50}}}, api.Request{})
	var seqs []int
	e.Sink = func(event api.Event) { seqs = append(seqs, event.Seq) }
	var wg sync.WaitGroup
	for _, id := range []string{"a", "b"} {
		wg.Go(func() {
			r := e.RunStep(context.Background(), api.Step{ID: id, Kind: api.KindAsk}, "codex", nil)
			if r.Result.Status != api.StatusCompleted {
				t.Errorf("parallel step: %+v", r)
			}
		})
	}
	wg.Wait()
	for i, seq := range seqs {
		if seq != i+1 {
			t.Fatalf("sink order: %v", seqs)
		}
	}
}

func TestRunStepCodeInTaskWorkdir(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK("created"), WriteFiles: map[string]string{"a.txt": "a"}}}}, api.Request{})
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindCode}, "codex", nil)
	if r.Result.Status != api.StatusCompleted || r.Changes == nil || r.Changes.Isolation != "none" {
		t.Fatalf("temporary workdir protocol isolation: %+v", r)
	}
}

func TestRunStepIncludesUpstreamArtifactDirs(t *testing.T) {
	e := stepEnv(t, testutil.FakeScript{"codex": {{Stdout: codexOK("ok")}}}, api.Request{})
	dir := t.TempDir()
	up := []recipe.Handoff{{StepID: "image", Artifacts: []string{filepath.Join(dir, "image.png")}}}
	r := e.RunStep(context.Background(), api.Step{ID: "main", Kind: api.KindAsk}, "codex", up)
	if r.Result.Status != api.StatusCompleted {
		t.Fatalf("handoff: %+v", r)
	}
	if !strings.Contains(strings.Join(testutil.FakeCalls(t)[0].Args, " "), "--add-dir "+dir) {
		t.Fatal("upstream artifact directory missing from CLI access roots")
	}
}

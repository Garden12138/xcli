package server

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/engine"
	"github.com/Garden12138/xcli/internal/testutil"
)

const testToken = "test-token"

func okLines(text string) []string {
	raw, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]string{"type": "agent_message", "text": text}})
	return []string{`{"type":"thread.started","thread_id":"server-session"}`, string(raw), `{"type":"turn.completed"}`}
}

func setup(t *testing.T, script testutil.FakeScript) (*httptest.Server, *engine.Engine, http.Handler) {
	t.Helper()
	home := t.TempDir()
	cfg := config.Default()
	cfg.Defaults.InheritShellProxy = false
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
	e, err := engine.New(home)
	if err != nil {
		t.Fatal(err)
	}
	h := New(e, testToken)
	ts := httptest.NewServer(h)
	t.Cleanup(func() {
		if err := h.(io.Closer).Close(); err != nil {
			t.Error(err)
		}
		ts.Close()
	})
	return ts, e, h
}

func request(t *testing.T, ts *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	client := *ts.Client()
	client.Timeout = 8 * time.Second
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func decode[T any](t *testing.T, res *http.Response, want int) T {
	t.Helper()
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != want {
		t.Fatalf("status=%d want=%d body=%s", res.StatusCode, want, raw)
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("JSON: %s %v", raw, err)
	}
	return v
}

func submit(t *testing.T, ts *httptest.Server, body string) string {
	t.Helper()
	r := decode[map[string]string](t, request(t, ts, "POST", "/v1/tasks", body), http.StatusAccepted)
	if r["task_id"] == "" {
		t.Fatal(r)
	}
	return r["task_id"]
}

func waitResult(t *testing.T, ts *httptest.Server, id string) api.Result {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		r := decode[api.Result](t, request(t, ts, "GET", "/v1/tasks/"+id, ""), 200)
		if r.Status.Finished() {
			return r
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("task did not finish")
	return api.Result{}
}

func waitCalls(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(testutil.FakeCalls(t)) >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("fake CLI did not start")
}

func TestHealthNoAuth(t *testing.T) {
	ts, _, _ := setup(t, testutil.FakeScript{})
	res, err := ts.Client().Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	v := decode[struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}](t, res, 200)
	if !v.OK || v.Version != "2.0.1" {
		t.Fatal(v)
	}
}

func TestAuthRequired(t *testing.T) {
	ts, _, _ := setup(t, testutil.FakeScript{})
	for _, token := range []string{"", "Bearer wrong", "test-token"} {
		req, _ := http.NewRequest("GET", ts.URL+"/v1/tasks", nil)
		req.Header.Set("Authorization", token)
		res, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 401 {
			t.Fatal(res.StatusCode)
		}
	}
}

func TestCreateAndGet(t *testing.T) {
	ts, e, _ := setup(t, testutil.FakeScript{"codex": {{Stdout: okLines("hello"), SleepMS: 80}}})
	id := submit(t, ts, `{"capability":"ask","agent":"codex","goal":"hi","metadata":{"app":"test"}}`)
	r := waitResult(t, ts, id)
	if r.Status != api.StatusCompleted || r.TaskID != id || r.Summary != "hello" || r.Metadata["app"] != "test" {
		t.Fatalf("result: %+v", r)
	}
	task, err := e.Store.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(task.Path("http")); err != nil {
		t.Fatal("missing HTTP task marker", err)
	}
	rows := decode[[]json.RawMessage](t, request(t, ts, "GET", "/v1/tasks?limit=1", ""), 200)
	if len(rows) != 1 {
		t.Fatal(rows)
	}
}

func TestCreateWait(t *testing.T) {
	ts, _, _ := setup(t, testutil.FakeScript{"codex": {{Stdout: okLines("waited")}}})
	r := decode[api.Result](t, request(t, ts, "POST", "/v1/tasks?wait=true", `{"capability":"ask","agent":"codex","goal":"hi"}`), 200)
	if r.Status != api.StatusCompleted || r.Summary != "waited" {
		t.Fatalf("wait: %+v", r)
	}
}

func TestQueuedResultHasEmptySteps(t *testing.T) {
	ts, e, _ := setup(t, testutil.FakeScript{})
	task, err := e.Create(api.Request{Capability: api.CapAsk, Goal: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	raw := decode[map[string]json.RawMessage](t, request(t, ts, "GET", "/v1/tasks/"+task.ID(), ""), 200)
	if string(raw["steps"]) != "[]" || string(raw["artifacts"]) != "[]" {
		t.Fatalf("queued arrays: %s %s", raw["steps"], raw["artifacts"])
	}
}

func TestInvalidRequest(t *testing.T) {
	ts, e, _ := setup(t, testutil.FakeScript{})
	for _, body := range []string{`{}`, `{"goal":"hi","unknown":true}`, `{"goal":"hi"}{"goal":"again"}`, `null`, `[]`} {
		r := decode[api.Result](t, request(t, ts, "POST", "/v1/tasks", body), 400)
		if r.Status != api.StatusInvalid || r.Error == nil || r.Error.Kind != "invalid_request" {
			t.Fatalf("invalid: %+v", r)
		}
	}
	rows, err := e.Store.List(0)
	if err != nil || len(rows) != 0 || len(testutil.FakeCalls(t)) != 0 {
		t.Fatalf("invalid persisted: %+v %v", rows, err)
	}
}

func TestEventsSSE(t *testing.T) {
	ts, _, _ := setup(t, testutil.FakeScript{"codex": {{Stdout: okLines("events"), SleepMS: 50}}})
	id := submit(t, ts, `{"capability":"ask","agent":"codex","goal":"hi"}`)
	for _, after := range []int{0, 2} {
		res := request(t, ts, "GET", "/v1/tasks/"+id+"/events?after="+strconv.Itoa(after), "")
		if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
			t.Fatal(res.Status, res.Header)
		}
		scanner := bufio.NewScanner(res.Body)
		var last api.Event
		var eventName, eventID string
		count := 0
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				eventName = strings.TrimPrefix(line, "event: ")
			}
			if strings.HasPrefix(line, "id: ") {
				eventID = strings.TrimPrefix(line, "id: ")
			}
			if strings.HasPrefix(line, "data: ") {
				var ev api.Event
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
					t.Fatal(err)
				}
				if ev.Seq <= after || ev.Seq <= last.Seq || ev.Type != eventName || eventID != strconv.Itoa(ev.Seq) {
					t.Fatalf("SSE: %+v", ev)
				}
				last = ev
				count++
			}
		}
		res.Body.Close()
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		if count == 0 || last.Type != "task.finished" {
			t.Fatalf("SSE terminal: %+v count=%d", last, count)
		}
		var result api.Result
		if json.Unmarshal(last.Data, &result) != nil || result.Status != api.StatusCompleted {
			t.Fatal(string(last.Data))
		}
		if after == 2 {
			res = request(t, ts, "GET", "/v1/tasks/"+id+"/events?after="+strconv.Itoa(last.Seq), "")
			data, err := io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil || len(data) != 0 {
				t.Fatalf("after terminal: %q %v", data, err)
			}
		}
	}
}

func TestCancel(t *testing.T) {
	ts, _, _ := setup(t, testutil.FakeScript{"codex": {{SleepMS: 10000, Stdout: okLines("late")}, {SleepMS: 150, Stdout: okLines("other")}}})
	a := submit(t, ts, `{"capability":"ask","agent":"codex","goal":"cancel me"}`)
	waitCalls(t, 1)
	b := submit(t, ts, `{"capability":"ask","agent":"codex","goal":"keep running"}`)
	waitCalls(t, 2)
	res := request(t, ts, "POST", "/v1/tasks/"+a+"/cancel", "")
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
	if r := waitResult(t, ts, a); r.Status != api.StatusCancelled {
		t.Fatalf("cancel: %+v", r)
	}
	if r := waitResult(t, ts, b); r.Status != api.StatusCompleted || r.Summary != "other" {
		t.Fatalf("unrelated task: %+v", r)
	}
	decode[map[string]any](t, request(t, ts, "GET", "/v1/health", ""), 200)
}

func TestCLIUsesHTTPGuidance(t *testing.T) {
	ts, e, _ := setup(t, testutil.FakeScript{"codex": {{SleepMS: 10000, Stdout: okLines("late")}}})
	id := submit(t, ts, `{"capability":"ask","agent":"codex","goal":"HTTP task"}`)
	waitCalls(t, 1)
	err := e.Cancel(id)
	if err == nil || !strings.Contains(err.Error(), "HTTP POST /v1/tasks/"+id+"/cancel") {
		t.Fatalf("CLI cancel guidance: %v", err)
	}
	r := decode[api.Result](t, request(t, ts, "GET", "/v1/tasks/"+id, ""), 200)
	if r.Status != api.StatusRunning {
		t.Fatalf("CLI signalled task: %+v", r)
	}
}

func TestArtifactDownload(t *testing.T) {
	ts, _, _ := setup(t, testutil.FakeScript{"codex": {{Stdout: okLines(`{"files":[]}`), WritePNG: "{{OUT}}/img.png"}}})
	r := decode[api.Result](t, request(t, ts, "POST", "/v1/tasks?wait=true", `{"capability":"image","agent":"codex","goal":"test image"}`), 200)
	if r.Status != api.StatusCompleted || len(r.Artifacts) != 1 {
		t.Fatalf("image: %+v", r)
	}
	res := request(t, ts, "GET", "/v1/tasks/"+r.TaskID+"/artifacts/0", "")
	data, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/png" || !bytes.HasPrefix(data, []byte{137, 80, 78, 71}) {
		t.Fatalf("artifact: %s %v", res.Status, err)
	}
	res = request(t, ts, "GET", "/v1/tasks/"+r.TaskID+"/artifacts/9", "")
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatal(res.Status)
	}
}

func TestCloseCancelsTasksAndStopsSubmissions(t *testing.T) {
	ts, e, h := setup(t, testutil.FakeScript{"codex": {{SleepMS: 10000, Stdout: okLines("late")}}})
	a := submit(t, ts, `{"capability":"ask","agent":"codex","goal":"a"}`)
	b := submit(t, ts, `{"capability":"ask","agent":"codex","goal":"b"}`)
	waitCalls(t, 2)
	if err := h.(io.Closer).Close(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{a, b} {
		r, err := e.Status(id)
		if err != nil || r.Status != api.StatusCancelled || r.Result == nil {
			t.Fatalf("shutdown: %+v %v", r, err)
		}
	}
	res := request(t, ts, "POST", "/v1/tasks", `{"capability":"ask","goal":"too late"}`)
	res.Body.Close()
	if res.StatusCode != 503 {
		t.Fatal(res.Status)
	}
	rows, _ := e.Store.List(0)
	if len(rows) != 2 {
		t.Fatal("shutdown created another task")
	}
}

func TestAgentsAndBadQueries(t *testing.T) {
	ts, _, _ := setup(t, testutil.FakeScript{})
	rows := decode[[]struct {
		Name      string `json:"name"`
		Installed bool   `json:"installed"`
	}](t, request(t, ts, "GET", "/v1/agents", ""), 200)
	if len(rows) != 3 {
		t.Fatal(rows)
	}
	for _, r := range rows {
		if !r.Installed {
			t.Fatal(r)
		}
	}
	for _, path := range []string{"/v1/tasks?limit=-1", "/v1/tasks?limit=bad", "/v1/tasks/missing/events?after=-1"} {
		res := request(t, ts, "GET", path, "")
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Fatalf("%s: %s", path, res.Status)
		}
	}
}

func TestLoadOrCreateToken(t *testing.T) {
	home := t.TempDir()
	var wg sync.WaitGroup
	tokens := make(chan string, 12)
	errs := make(chan error, 12)
	for range 12 {
		wg.Go(func() { token, err := LoadOrCreateToken(home); tokens <- token; errs <- err })
	}
	wg.Wait()
	close(tokens)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first string
	for token := range tokens {
		if first == "" {
			first = token
		}
		if token != first {
			t.Fatal("token changed during concurrent creation")
		}
	}
	if raw, err := hex.DecodeString(first); err != nil || len(raw) != 32 {
		t.Fatalf("token length: %v", err)
	}
	info, err := os.Stat(filepath.Join(home, "server.token"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("token mode: %v %v", info, err)
	}
	if err := os.WriteFile(filepath.Join(home, "server.token"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateToken(home); err == nil {
		t.Fatal("invalid token accepted")
	}
}

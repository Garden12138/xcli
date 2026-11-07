package agentgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/agentservice"
	"github.com/Garden12138/xcli/internal/agentstore"
	"github.com/Garden12138/xcli/internal/nativeadapter"
)

func setup(t *testing.T) (*Server, string) {
	t.Helper()
	st, e := agentstore.Open(filepath.Join(t.TempDir(), "state"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Close() })
	cs := &agentstore.Clients{Root: st.Root}
	token, e := cs.Issue(agentstore.Client{ID: "worklog", Scopes: agentstore.Scopes, Agents: []string{"fake"}, Operations: []string{"text_task"}, Policies: []string{"test-only"}, AllowRemote: true, DailyDispatches: 6, DailyProcessStarts: 6, DailyExecutionSeconds: 3600}, false)
	if e != nil {
		t.Fatal(e)
	}
	cfg := agentpolicy.DefaultConfig()
	cfg.DailyDispatches = 6
	cfg.DailyProcessStarts = 6
	cfg.DailyExecutionSeconds = 3600
	p := agentpolicy.Profile{AgentID: "fake", AdapterID: "fake-v1", PolicyID: "test-only", Revision: "1", CapabilityRevision: "fake-cap-1", Location: "remote", Operations: []string{"text_task"}}.Freeze()
	svc, e := agentservice.New(st, cs, cfg, []agentpolicy.Profile{p})
	if e != nil {
		t.Fatal(e)
	}
	return &Server{Service: svc, Address: "127.0.0.1:18742"}, token
}
func call(s *Server, token, method, path, key string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+s.Address+path, bytes.NewReader(body))
	r.URL.Scheme = ""
	r.URL.Host = ""
	r.Header.Set("Authorization", "Bearer "+token)
	if method == "POST" {
		r.Header.Set("Idempotency-Key", key)
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func encode(v any) []byte { b, _ := json.Marshal(v); return b }
func TestHTTPContractAndSecurityBoundary(t *testing.T) {
	s, token := setup(t)
	if w := call(s, "", "GET", "/agent/v1/capabilities", "", nil); w.Code != 401 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(s, token, "GET", "/v1/models", "", nil); w.Code != 404 {
		t.Fatal(w.Code)
	}
	for _, h := range []string{"Origin", "Forwarded", "X-Forwarded-Host", "Via", "Proxy-Authorization"} {
		r := httptest.NewRequest("GET", "/agent/v1/capabilities", nil)
		r.Host = s.Address
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(h, "injected")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(h, w.Code)
		}
	}
	for _, host := range []string{"localhost:18742", "127.0.0.2:18742", "evil.example:18742", "127.0.0.1:9999"} {
		r := httptest.NewRequest("GET", "/healthz", nil)
		r.Host = host
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(host, w.Code)
		}
	}
	for _, body := range []string{`{"agent_id":"fake","agent_id":"codex"}`, `{"env":{}}`, `{} {}`, `{"bad":"\ud800"}`} {
		w := call(s, token, "POST", "/agent/v1/sessions", "strict-key", []byte(body))
		if w.Code < 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := call(s, token, "GET", "/agent/v1/capabilities", "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "offline_fixture_only") || strings.Contains(w.Body.String(), token) {
		t.Fatal(w.Code, w.Body.String())
	}
}

type httpDriver struct{ calls int }

func (d *httpDriver) StartTurn(_ context.Context, b nativeadapter.BoundRequest, o func(nativeadapter.Observation) error) nativeadapter.TurnResult {
	d.calls++
	_ = o(nativeadapter.Observation{Category: "response"})
	zero := 0
	return nativeadapter.TurnResult{Text: "HTTP fixture result", NativeID: "fixture-native-id", SessionObserved: true, ExitCode: &zero, Terminal: "success", Protocol: "complete", LocalStopConfirmed: true}
}
func TestHTTPToFakeDriverAndSSE(t *testing.T) {
	s, token := setup(t)
	w := call(s, token, "POST", "/agent/v1/sessions", "session-key", encode(api.SessionCreate{AgentID: "fake", AdapterID: "fake-v1", PolicyID: "test-only", CapabilityRevision: s.Service.Profiles[0].CapabilityRevision, Budget: api.Budget{MaxDispatches: 2, MaxProcessStarts: 2, MaxExecutionSeconds: 300}, Context: api.Context{App: "worklog", Workflow: "draft", OpaqueRef: "synthetic"}}))
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var ses api.Session
	_ = json.Unmarshal(w.Body.Bytes(), &ses)
	text := "synthetic task"
	req := api.TaskCreate{SessionID: ses.ID, ExpectedSessionRevision: ses.Revision, Operation: "text_task", Input: []api.Part{{Type: "text", Text: &text}}, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, TimeoutSeconds: 120}, Consent: api.Consent{AllowRemote: true, ContextSHA256: ses.ContextSHA256}}
	w = call(s, token, "POST", "/agent/v1/tasks", "task-key-1", encode(req))
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var task api.Task
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	d := &httpDriver{}
	if _, e := s.Service.ExecuteNext(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	w = call(s, token, "GET", "/agent/v1/tasks/"+task.ID, "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"succeeded"`) || strings.Contains(w.Body.String(), "fixture-native-id") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call(s, token, "POST", "/agent/v1/tasks", "task-key-1", encode(req))
	if w.Code != 202 || w.Header().Get("Idempotency-Replayed") != "true" || d.calls != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call(s, token, "GET", "/agent/v1/tasks/"+task.ID+"/events", "", nil)
	if w.Code != 200 || strings.Count(w.Body.String(), "event: task.completed") != 1 || strings.Contains(w.Body.String(), "text.delta") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestLoopbackHTTPClientCapabilities(t *testing.T) {
	s, token := setup(t)
	server := httptest.NewServer(s)
	defer server.Close()
	s.Address = strings.TrimPrefix(server.URL, "http://")
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest("GET", server.URL+api.Prefix+"/capabilities", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !bytes.Contains(body, []byte("fake-v1")) {
		t.Fatal(res.StatusCode, string(body))
	}
}

package nativeadapter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentgateway"
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/agentservice"
	"github.com/Garden12138/xcli/internal/agentstore"
	"github.com/Garden12138/xcli/internal/modelstore"
	"github.com/Garden12138/xcli/internal/nativeadapter"
)

// This fixture is explicitly non-native, injected only through Go tests. No
// production config can construct it or bypass nativeToolBoundaryVerified.
func limitedService(t *testing.T, sleep bool) (*agentservice.Service, agentstore.Client, string, string) {
	t.Helper()
	old, c := fixture(t)
	p := old.Profiles[0]
	body, e := os.ReadFile(p.Binary)
	if e != nil {
		t.Fatal(e)
	}
	prefix := "#!/bin/sh\nif [ \"$1\" = --version ]; then printf 'codex-cli 0.167.0\\n'; exit; fi\n"
	if sleep {
		prefix += "export XCLI_NAG_FORCE_SLEEP=1\n"
	}
	body = []byte(strings.Replace(string(body), "#!/bin/sh\n", prefix, 1))
	if e = os.WriteFile(p.Binary, body, 0700); e != nil {
		t.Fatal(e)
	}
	auth, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	r, e := agentpolicy.PrepareLimited(context.Background(), old.Store.Root, p.Binary, auth)
	if e != nil {
		t.Fatal(e)
	}
	control := map[string]any{"registration_sha256": api.Digest(r), "enabled": true, "authorization_ref": "offline-fixture-only", "expires_at": time.Now().UTC().Add(time.Hour)}
	b, _ := json.Marshal(control)
	if e = modelstore.WritePrivate(filepath.Join(old.Store.Root, "codex-limited-control.json"), b); e != nil {
		t.Fatal(e)
	}
	c.Agents = []string{"codex"}
	c.Policies = []string{agentpolicy.LimitedPolicy}
	c.DailyDispatches = 1
	c.DailyProcessStarts = 1
	c.DailyExecutionSeconds = 120
	c.ID = "limited-owner-fixture"
	token, e := old.Clients.Issue(c, false)
	if e != nil {
		t.Fatal(e)
	}
	c, e = old.Clients.Get(c.ID)
	if e != nil {
		t.Fatal(e)
	}
	p.AgentID = "codex"
	p.PolicyID = agentpolicy.LimitedPolicy
	p.BinarySHA256 = r.BinarySHA256
	p.Limited = &agentpolicy.LimitedBinding{Root: old.Store.Root, RegistrationSHA256: api.Digest(r), ClientID: c.ID, Workspace: r.Workspace, AuthHome: auth}
	s, e := agentservice.New(old.Store, old.Clients, old.Config, []agentpolicy.Profile{p})
	if e != nil {
		t.Fatal(e)
	}
	return s, c, token, r.Workspace
}
func limitedSession(t *testing.T, s *agentservice.Service, c agentstore.Client) api.Session {
	t.Helper()
	p := s.Profiles[0]
	out, _, e := s.CreateSession(c, api.SessionCreate{AgentID: p.AgentID, AdapterID: p.AdapterID, PolicyID: p.PolicyID, CapabilityRevision: p.CapabilityRevision, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, MaxExecutionSeconds: 120}, Context: api.Context{App: "worklog", Workflow: "codex-first-http", OpaqueRef: "synthetic"}}, api.ID("key"))
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func limitedRequest(ses api.Session) api.TaskCreate {
	text := agentpolicy.LimitedPrompt
	return api.TaskCreate{SessionID: ses.ID, ExpectedSessionRevision: ses.Revision, Operation: "text_task", Input: []api.Part{{Type: "text", Text: &text}}, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, TimeoutSeconds: 120}, Consent: api.Consent{AllowRemote: true, ContextSHA256: ses.ContextSHA256}}
}
func TestLimitedHTTPFixtureOwnershipOneLifetimeStartAndPersistence(t *testing.T) {
	s, c, token, _ := limitedService(t, false)
	ses := limitedSession(t, s, c)
	handler := &agentgateway.Server{Service: s}
	server := httptest.NewServer(handler)
	handler.Address = strings.TrimPrefix(server.URL, "http://")
	defer server.Close()
	request := func(token string) (api.Task, int) {
		data, _ := json.Marshal(limitedRequest(ses))
		req, _ := http.NewRequest("POST", server.URL+"/agent/v1/tasks", strings.NewReader(string(data)))
		req.Header.Set("Host", strings.TrimPrefix(server.URL, "http://"))
		req.Host = strings.TrimPrefix(server.URL, "http://")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "limited-http-one-key")
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Error(e)
			return api.Task{}, 0
		}
		defer res.Body.Close()
		var out api.Task
		_ = json.NewDecoder(res.Body).Decode(&out)
		return out, res.StatusCode
	}
	if _, status := request("invalid"); status != 401 {
		t.Fatal(status)
	}
	var wg sync.WaitGroup
	ids := make(chan string, 100)
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, status := request(token)
			if status == 429 {
				return
			} // documented HTTP admission limit; zero extra Task
			if status != 202 {
				t.Error(status)
			}
			ids <- out.ID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for next := range ids {
		if id != "" && id != next {
			t.Fatal("duplicate tasks")
		}
		id = next
	}
	if _, e := s.ExecuteNext(context.Background(), nativeadapter.Exec{}); e != nil {
		t.Fatal(e)
	}
	task, e := s.GetTask(c, id)
	if e != nil || task.Status != "succeeded" || !task.Result.Final {
		t.Fatal(task, e)
	}
	events, _, _, e := s.Events(c, id, 0)
	if e != nil || len(events) == 0 {
		t.Fatal(e)
	}
	other := c
	other.ID = "other"
	if _, e = s.GetTask(other, id); e == nil {
		t.Fatal("cross owner read")
	}
	if _, e = s.GetSession(other, ses.ID); e == nil {
		t.Fatal("cross owner session")
	}
	next := limitedSession(t, s, c)
	if _, _, e = s.CreateTask(c, limitedRequest(next), api.ID("key")); e == nil {
		t.Fatal("second lifetime dispatch accepted")
	}
	root := s.Store.Root
	if e = s.Store.Close(); e != nil {
		t.Fatal(e)
	}
	st, e := agentstore.Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	recovered, e := agentservice.New(st, s.Clients, s.Config, s.Profiles)
	if e != nil {
		t.Fatal(e)
	}
	task, e = recovered.GetTask(c, id)
	if e != nil || task.Status != "succeeded" {
		t.Fatal(task, e)
	}
	if _, _, e = recovered.CreateTask(c, limitedRequest(next), api.ID("key")); e == nil {
		t.Fatal("restart refunded lifetime start")
	}
}
func TestLimitedFixtureDisableRevokeTimeoutAndLateSuccess(t *testing.T) {
	for _, mode := range []string{"disable", "revoke", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			s, c, _, _ := limitedService(t, true)
			ses := limitedSession(t, s, c)
			task, _, e := s.CreateTask(c, limitedRequest(ses), api.ID("key"))
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if mode == "timeout" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 300*time.Millisecond)
			}
			defer cancel()
			done := make(chan error, 1)
			go func() { _, e := s.ExecuteNext(ctx, nativeadapter.Exec{}); done <- e }()
			until := time.Now().Add(2 * time.Second)
			for time.Now().Before(until) {
				out, _ := s.GetTask(c, task.ID)
				if out.Status == "running" {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if mode == "disable" {
				if e = agentpolicy.DisableLimited(s.Store.Root); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "revoke" {
				if e = s.Clients.Revoke(c.ID); e != nil {
					t.Fatal(e)
				}
			}
			if e = <-done; e != nil {
				t.Fatal(e)
			}
			out, e := s.GetTask(c, task.ID)
			if e != nil || out.Status == "succeeded" || out.Result.Final {
				t.Fatal(out, e)
			}
			if mode == "timeout" && out.Status != "timed_out" {
				t.Fatal(out)
			}
		})
	}
}

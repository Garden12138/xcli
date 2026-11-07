package agentgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Garden12138/xcli/internal/agentservice"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentstore"
)

func eventTask(t *testing.T, s *Server, token string) api.Task {
	t.Helper()
	p := s.Service.Profiles[0]
	w := call(s, token, "POST", api.Prefix+"/sessions", "events-session", encode(api.SessionCreate{AgentID: p.AgentID, AdapterID: p.AdapterID, PolicyID: p.PolicyID, CapabilityRevision: p.CapabilityRevision, Budget: api.Budget{MaxDispatches: 2, MaxProcessStarts: 2, MaxExecutionSeconds: 300}, Context: api.Context{App: "worklog", Workflow: "synthetic", OpaqueRef: "events"}}))
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var ses api.Session
	_ = json.Unmarshal(w.Body.Bytes(), &ses)
	text := "fixture"
	w = call(s, token, "POST", api.Prefix+"/tasks", "events-task", encode(api.TaskCreate{SessionID: ses.ID, ExpectedSessionRevision: ses.Revision, Operation: "text_task", Input: []api.Part{{Type: "text", Text: &text}}, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, TimeoutSeconds: 120}, Consent: api.Consent{AllowRemote: true, ContextSHA256: ses.ContextSHA256}}))
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var task api.Task
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	return task
}
func TestSSEReconnectCursorValidationAndRevocation(t *testing.T) {
	s, token := setup(t)
	task := eventTask(t, s, token)
	d := &httpDriver{}
	if _, e := s.Service.ExecuteNext(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	request := func(cursor string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", api.Prefix+"/tasks/"+task.ID+"/events", nil)
		r.Host = s.Address
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Last-Event-ID", cursor)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	w := request("2")
	if w.Code != 200 || strings.Contains(w.Body.String(), "id: 1\n") || strings.Count(w.Body.String(), "event: task.completed") != 1 || d.calls != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, bad := range []string{"-1", "99999", "abc"} {
		if w = request(bad); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	if e := s.Service.Store.Update(func(st *agentstore.State) error {
		r := st.Tasks[task.ID]
		r.Events = r.Events[len(r.Events)-1:]
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	w = request("0")
	if w.Code != 410 || w.Header().Get("X-Xcli-Latest-Event-ID") == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := s.Service.Clients.Revoke("worklog"); e != nil {
		t.Fatal(e)
	}
	w = request("2")
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestSSEDisconnectDoesNotCancelQueuedTask(t *testing.T) {
	s, token := setup(t)
	task := eventTask(t, s, token)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest("GET", api.Prefix+"/tasks/"+task.ID+"/events", nil).WithContext(ctx)
	r.Host = s.Address
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	d := &httpDriver{}
	if _, e := s.Service.ExecuteNext(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	c, _ := s.Service.Clients.Get("worklog")
	out, e := s.Service.GetTask(c, task.ID)
	if e != nil || out.Status != "succeeded" || d.calls != 1 {
		t.Fatal(out, e, d.calls)
	}
}
func TestEventRetentionKeepsContiguousTailAndTerminal(t *testing.T) {
	s, token := setup(t)
	task := eventTask(t, s, token)
	if e := s.Service.Store.Update(func(st *agentstore.State) error {
		r := st.Tasks[task.ID]
		r.Events = nil
		for i := int64(2); i <= 10002; i++ {
			r.Events = append(r.Events, api.Event{APIVersion: 1, TaskID: task.ID, SessionID: task.SessionID, Seq: i, Time: time.Now().UTC(), Type: "task.progress", Payload: map[string]any{"category": "activity"}})
		}
		r.Task.LastEventSeq = 10002
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	d := &httpDriver{}
	if _, e := s.Service.ExecuteNext(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	_ = s.Service.Store.View(func(st agentstore.State) error {
		events := st.Tasks[task.ID].Events
		if len(events) > 10000 || events[len(events)-1].Type != "task.completed" {
			t.Fatal(len(events))
		}
		for i := 1; i < len(events); i++ {
			if events[i].Seq != events[i-1].Seq+1 {
				t.Fatal("sequence gap")
			}
		}
		return nil
	})
}

type slowSSEWriter struct {
	header   http.Header
	deadline time.Time
	status   int
}

func (w *slowSSEWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (w *slowSSEWriter) WriteHeader(status int)                    { w.status = status }
func (w *slowSSEWriter) SetWriteDeadline(deadline time.Time) error { w.deadline = deadline; return nil }
func (w *slowSSEWriter) Flush()                                    {}
func (w *slowSSEWriter) Write([]byte) (int, error) {
	time.Sleep(max(0, time.Until(w.deadline)))
	return 0, os.ErrDeadlineExceeded
}
func TestSlowSSEReaderCannotOwnWorker(t *testing.T) {
	s, token := setup(t)
	task := eventTask(t, s, token)
	r := httptest.NewRequest("GET", api.Prefix+"/tasks/"+task.ID+"/events", nil)
	r.Host = s.Address
	r.Header.Set("Authorization", "Bearer "+token)
	w := &slowSSEWriter{}
	start := time.Now()
	s.ServeHTTP(w, r)
	if time.Since(start) > 2*time.Second {
		t.Fatal("slow observer did not disconnect")
	}
	d := &httpDriver{}
	if _, e := s.Service.ExecuteNext(context.Background(), d); e != nil || d.calls != 1 {
		t.Fatal(e, d.calls)
	}
}
func Test100ConcurrentHTTPIdempotencyHasOneTaskAndDispatch(t *testing.T) {
	s, token := setup(t)
	task := eventTask(t, s, token) // use a fresh session because eventTask already accepted one task
	c, _ := s.Service.Clients.Get("worklog")
	_, _, _ = s.Service.Cancel(c, task.ID, "cancel-prior")
	ses, e := s.Service.GetSession(c, task.SessionID)
	if e != nil {
		t.Fatal(e)
	}
	text := "one synthetic HTTP turn"
	payload := encode(api.TaskCreate{SessionID: ses.ID, ExpectedSessionRevision: ses.Revision, Operation: "text_task", Input: []api.Part{{Type: "text", Text: &text}}, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, TimeoutSeconds: 120}, Consent: api.Consent{AllowRemote: true, ContextSHA256: ses.ContextSHA256}})
	server := httptest.NewServer(s)
	defer server.Close()
	s.Address = strings.TrimPrefix(server.URL, "http://")
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	ids := make(chan string, 100)
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, _ := http.NewRequest("POST", server.URL+api.Prefix+"/tasks", bytes.NewReader(payload))
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Idempotency-Key", "http-one-key")
			res, e := client.Do(r)
			if e != nil {
				t.Error(e)
				return
			}
			defer res.Body.Close()
			b, _ := io.ReadAll(res.Body)
			if res.StatusCode == 429 {
				return
			}
			if res.StatusCode != 202 {
				t.Errorf("%d %s", res.StatusCode, b)
				return
			}
			var out api.Task
			_ = json.Unmarshal(b, &out)
			ids <- out.ID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for v := range ids {
		if id != "" && id != v {
			t.Fatal("duplicate HTTP Task")
		}
		id = v
	}
	if id == "" {
		t.Fatal("all HTTP submissions rejected")
	}
	d := &httpDriver{}
	if _, e = s.Service.ExecuteNext(context.Background(), d); e != nil || d.calls != 1 {
		t.Fatal(e, d.calls)
	}
	_ = s.Service.Store.View(func(st agentstore.State) error {
		if len(st.Tasks) != 2 {
			t.Fatal("created duplicate tasks", len(st.Tasks))
		}
		return nil
	})
}
func TestAmbiguousFsyncAbortsHTTPInsteadOfFalseRejection(t *testing.T) {
	s, token := setup(t)
	task := eventTask(t, s, token)
	c, _ := s.Service.Clients.Get("worklog")
	_, _, _ = s.Service.Cancel(c, task.ID, "cancel-initial")
	ses, _ := s.Service.GetSession(c, task.SessionID)
	text := "ambiguous accept"
	request := api.TaskCreate{SessionID: ses.ID, ExpectedSessionRevision: ses.Revision, Operation: "text_task", Input: []api.Part{{Type: "text", Text: &text}}, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, TimeoutSeconds: 120}, Consent: api.Consent{AllowRemote: true, ContextSHA256: ses.ContextSHA256}}
	s.Service.Store.BeforeSync = func() error { return syscall.ENOSPC }
	server := httptest.NewServer(s)
	s.Address = strings.TrimPrefix(server.URL, "http://")
	r, _ := http.NewRequest("POST", server.URL+api.Prefix+"/tasks", bytes.NewReader(encode(request)))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", "ambiguous-disk-full")
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 3 * time.Second}
	res, e := client.Do(r)
	if e == nil {
		res.Body.Close()
		t.Fatal("ambiguous mutation was acknowledged")
	}
	server.Close()
	root := s.Service.Store.Root
	_ = s.Service.Store.Close()
	st, e := agentstore.Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	svc, e := agentservice.New(st, s.Service.Clients, s.Service.Config, s.Service.Profiles)
	if e != nil {
		t.Fatal(e)
	}
	d := &httpDriver{}
	if _, e = svc.ExecuteNext(context.Background(), d); e != nil || d.calls != 0 {
		t.Fatal("ambiguous accept dispatched", e, d.calls)
	}
}

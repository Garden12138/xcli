package nativeadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Garden12138/xcli/internal/agentgateway"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/agentservice"
	"github.com/Garden12138/xcli/internal/agentstore"
	"github.com/Garden12138/xcli/internal/nativeadapter"
)

func TestGatewayCLIHelper(t *testing.T) {
	if os.Getenv("XCLI_NAG_HELPER") != "1" {
		return
	}
	args := os.Args
	prompt := strings.TrimPrefix(args[len(args)-1], "xcli user task:\n")
	if adapter := os.Getenv("XCLI_NAG_ADAPTER"); adapter != "" {
		if adapter == "cursor" && prompt == "trust" {
			fmt.Fprintln(os.Stderr, "Workspace is not trusted")
			os.Exit(1)
		}
		if adapter == "cursor" {
			fmt.Println(`{"type":"system","session_id":"fixture-print"}`)
			fmt.Println(`{"type":"assistant","message":{"role":"assistant","content":"fixture result"}}`)
			fmt.Println(`{"type":"result","subtype":"success","session_id":"fixture-print"}`)
		} else {
			denied := "[]"
			if prompt == "soft-deny" {
				denied = `[{"tool":"command"}]`
			}
			fmt.Printf("{\"event\":\"result\",\"result\":{\"conversation_id\":\"fixture-print\",\"status\":\"SUCCESS\",\"response\":\"fixture result\",\"denied_actions\":%s}}\n", denied)
		}
		os.Exit(0)
	}
	id := "fixture-thread"
	if slices.Contains(args, "resume") {
		id = args[len(args)-2]
	}
	if prompt == "lost" {
		id = "different-thread"
	}
	if prompt != "missing" {
		fmt.Printf("{\"type\":\"thread.started\",\"thread_id\":%q}\n", id)
	}
	if prompt == "sleep" {
		signal.Ignore(syscall.SIGTERM)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	if prompt == "quota" {
		fmt.Println(`{"type":"error","code":"usage_limit_exceeded"}`)
		os.Exit(1)
	}
	if prompt == "bad" {
		fmt.Println("{broken")
		os.Exit(0)
	}
	if prompt == "unknown-output" {
		fmt.Println(`{"type":"unknown","result":"untrusted diagnostic text"}`)
		fmt.Println(`{"type":"turn.completed"}`)
		os.Exit(0)
	}
	if prompt == "noterminal" {
		fmt.Println(`{"type":"unknown"}`)
		os.Exit(0)
	}
	if prompt == "overlong" {
		fmt.Printf("{\"type\":\"unknown\",\"data\":\"%s\"}\n", strings.Repeat("x", 9<<20))
		os.Exit(0)
	}
	if os.Getenv("OPENAI_API_KEY") != "" || os.Getenv("HTTP_PROXY") != "" || os.Getenv("XCLI_APPLICATION_TOKEN") != "" {
		os.Exit(11)
	}
	text := "fixture result"
	if prompt == "split" {
		out := `{"type":"item.completed","item":{"type":"agent_message","text":"fixture result"}}` + "\n"
		for _, b := range []byte(out) {
			_, _ = os.Stdout.Write([]byte{b})
		}
	} else {
		fmt.Printf("{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":%q}}\n", text)
	}
	fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":0,"output_tokens":2}}`)
	os.Exit(0)
}
func fixture(t *testing.T) (*agentservice.Service, agentstore.Client) {
	t.Helper()
	root := t.TempDir()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	script := filepath.Join(root, "fixture-cli")
	body := []byte("#!/bin/sh\nexport XCLI_NAG_HELPER=1\nexport GORACE=atexit_sleep_ms=0\nexec '" + strings.ReplaceAll(exe, "'", "'\\''") + "' -test.run=^TestGatewayCLIHelper$ -- \"$@\"\n")
	if e = os.WriteFile(script, body, 0o700); e != nil {
		t.Fatal(e)
	}
	st, e := agentstore.Open(filepath.Join(root, "state"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Close() })
	clients := &agentstore.Clients{Root: st.Root}
	c := agentstore.Client{ID: "worklog", Scopes: agentstore.Scopes, Agents: []string{"fixture"}, Policies: []string{"offline-only"}, Operations: []string{"text_task"}, AllowRemote: true, DailyDispatches: 6, DailyProcessStarts: 6, DailyExecutionSeconds: 3600}
	if _, e = clients.Issue(c, false); e != nil {
		t.Fatal(e)
	}
	cfg := agentpolicy.DefaultConfig()
	cfg.DailyDispatches = 6
	cfg.DailyProcessStarts = 6
	cfg.DailyExecutionSeconds = 3600
	p := agentpolicy.Profile{AgentID: "fixture", AdapterID: "codex-exec-v1", PolicyID: "offline-only", Revision: "1", InstallationDomain: "fixture", Location: "remote", Binary: script, BinarySHA256: api.Hash(body), Operations: []string{"text_task"}}
	s, e := agentservice.New(st, clients, cfg, []agentpolicy.Profile{p})
	if e != nil {
		t.Fatal(e)
	}
	return s, c
}
func newSession(t *testing.T, s *agentservice.Service, c agentstore.Client) api.Session {
	t.Helper()
	p := s.Profiles[0]
	ses, _, e := s.CreateSession(c, api.SessionCreate{AgentID: p.AgentID, AdapterID: p.AdapterID, CapabilityRevision: p.CapabilityRevision, PolicyID: p.PolicyID, Budget: api.Budget{MaxDispatches: 2, MaxProcessStarts: 2, MaxExecutionSeconds: 300}, Context: api.Context{App: "worklog", Workflow: "synthetic", OpaqueRef: "fixture"}}, api.ID("key"))
	if e != nil {
		t.Fatal(e)
	}
	return ses
}
func create(t *testing.T, s *agentservice.Service, c agentstore.Client, ses api.Session, prompt string, timeout int) api.Task {
	t.Helper()
	out, _, e := s.CreateTask(c, api.TaskCreate{SessionID: ses.ID, ExpectedSessionRevision: ses.Revision, Operation: "text_task", Input: []api.Part{{Type: "text", Text: &prompt}}, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, TimeoutSeconds: timeout}, Consent: api.Consent{AllowRemote: true, ContextSHA256: ses.ContextSHA256}}, api.ID("key"))
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestNativeExecFixtureResumeAndProtocolFailures(t *testing.T) {
	for _, mode := range []string{"split", "lost", "missing", "bad", "noterminal", "unknown-output", "overlong", "quota"} {
		t.Run(mode, func(t *testing.T) {
			s, c := fixture(t)
			ses := newSession(t, s, c)
			out := create(t, s, c, ses, "first", 10)
			if _, e := s.ExecuteNext(context.Background(), nativeadapter.Exec{}); e != nil {
				t.Fatal(e)
			}
			out, _ = s.GetTask(c, out.ID)
			if out.Status != "succeeded" || !out.Protocol.NativeSessionObserved || out.Usage.InputTokens == nil || *out.Usage.InputTokens != 0 || out.Usage.TotalTokens != nil {
				t.Fatal(out)
			}
			ses, _ = s.GetSession(c, ses.ID)
			out = create(t, s, c, ses, mode, 10)
			if _, e := s.ExecuteNext(context.Background(), nativeadapter.Exec{}); e != nil {
				t.Fatal(e)
			}
			out, _ = s.GetTask(c, out.ID)
			if mode == "split" {
				if out.Status != "succeeded" {
					t.Fatal(out)
				}
			} else if out.Status == "succeeded" {
				b, _ := json.Marshal(out)
				t.Fatal(string(b))
			}
			if mode == "quota" && out.Error.Code != "native_quota_exceeded" {
				t.Fatal(out)
			}
			if mode == "lost" && out.Error.Code != "native_session_lost" {
				t.Fatal(out)
			}
		})
	}
}
func TestNativeExecFixtureTimeoutCancelAndRevocation(t *testing.T) {
	for _, mode := range []string{"timeout", "cancel", "revoke"} {
		t.Run(mode, func(t *testing.T) {
			s, c := fixture(t)
			ses := newSession(t, s, c)
			timeout := 10
			if mode == "timeout" {
				timeout = 1
			}
			out := create(t, s, c, ses, "sleep", timeout)
			done := make(chan error, 1)
			start := time.Now()
			go func() { _, e := s.ExecuteNext(context.Background(), nativeadapter.Exec{}); done <- e }()
			if mode != "timeout" {
				deadline := time.Now().Add(2 * time.Second)
				for time.Now().Before(deadline) {
					current, _ := s.GetTask(c, out.ID)
					if current.LastEventSeq >= 3 {
						break
					}
					time.Sleep(5 * time.Millisecond)
				}
				if mode == "cancel" {
					if _, _, e := s.Cancel(c, out.ID, "cancel-fixture"); e != nil {
						t.Fatal(e)
					}
				} else {
					if e := s.Clients.Revoke(c.ID); e != nil {
						t.Fatal(e)
					}
				}
			}
			select {
			case e := <-done:
				if e != nil {
					t.Fatal(e)
				}
			case <-time.After(7 * time.Second):
				t.Fatal("cleanup exceeded bound")
			}
			out, _ = s.GetTask(c, out.ID)
			if !api.Terminal(out.Status) || out.Status == "succeeded" || time.Since(start) > time.Duration(timeout)*time.Second+5*time.Second {
				t.Fatal(out, time.Since(start))
			}
			if out.Execution.LocalStopConfirmed == nil || !*out.Execution.LocalStopConfirmed {
				t.Fatal(out)
			}
			if out.Execution.UpstreamCancelConfirmed != nil || out.Execution.SideEffects != "unknown" {
				t.Fatal("invented cloud cancellation", out)
			}
			ses, _ = s.GetSession(c, ses.ID)
			if ses.State != "needs_reconcile" {
				t.Fatal(ses)
			}
		})
	}
}
func TestFixedNativeArgumentsDoNotEscalate(t *testing.T) {
	for _, id := range []string{"codex-exec-v1", "cursor-headless-v1", "antigravity-print-v1"} {
		for _, native := range []string{"", "fixture-id"} {
			b := nativeadapter.BoundRequest{Prompt: "--yolo", NativeID: native, Profile: agentpolicy.Profile{AgentID: "fixture", AdapterID: id, Binary: "/fixture"}}
			_, spec, e := nativeadapter.Specification(b)
			if e != nil {
				t.Fatal(e)
			}
			if strings.HasPrefix(spec.Args[len(spec.Args)-1], "-") {
				t.Fatal("prompt interpreted as native option", spec)
			}
			for _, arg := range spec.Args {
				if slices.Contains([]string{"--force", "--yolo", "--trust", "--dangerously-bypass-approvals-and-sandbox", "danger-full-access", "workspace-write"}, arg) {
					t.Fatal(spec)
				}
			}
		}
	}
}
func TestLoopbackHTTPToCLIToPersistedResult(t *testing.T) {
	s, c := fixture(t)
	token, e := s.Clients.Issue(c, true)
	if e != nil {
		t.Fatal(e)
	}
	server := &agentgateway.Server{Service: s}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	server.Address = strings.TrimPrefix(httpServer.URL, "http://")
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 10 * time.Second}
	request := func(method, path, key string, v any) []byte {
		t.Helper()
		var data []byte
		if v != nil {
			data, _ = json.Marshal(v)
		}
		req, _ := http.NewRequest(method, httpServer.URL+api.Prefix+path, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+token)
		if v != nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", key)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode >= 400 {
			t.Fatalf("%d %s", res.StatusCode, body)
		}
		return body
	}
	caps := api.Capabilities{}
	_ = json.Unmarshal(request("GET", "/capabilities", "", nil), &caps)
	p := s.Profiles[0]
	ses := api.Session{}
	_ = json.Unmarshal(request("POST", "/sessions", "http-session", api.SessionCreate{AgentID: p.AgentID, AdapterID: p.AdapterID, CapabilityRevision: caps.Revision, PolicyID: p.PolicyID, Budget: api.Budget{MaxDispatches: 2, MaxProcessStarts: 2, MaxExecutionSeconds: 300}, Context: api.Context{App: "worklog", Workflow: "synthetic", OpaqueRef: "http"}}), &ses)
	prompt := "split"
	req := api.TaskCreate{SessionID: ses.ID, ExpectedSessionRevision: ses.Revision, Operation: "text_task", Input: []api.Part{{Type: "text", Text: &prompt}}, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, TimeoutSeconds: 10}, Consent: api.Consent{AllowRemote: true, ContextSHA256: ses.ContextSHA256}}
	task := api.Task{}
	_ = json.Unmarshal(request("POST", "/tasks", "http-task", req), &task)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx, nativeadapter.Exec{}) }()
	for range 200 {
		_ = json.Unmarshal(request("GET", "/tasks/"+task.ID, "", nil), &task)
		if api.Terminal(task.Status) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if task.Status != "succeeded" || task.Result.Text == nil || *task.Result.Text != "fixture result" {
		t.Fatal(task)
	}
	events := request("GET", "/tasks/"+task.ID+"/events", "", nil)
	if bytes.Count(events, []byte("event: task.completed")) != 1 || bytes.Contains(events, []byte("text.delta")) {
		t.Fatal(string(events))
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
	_ = st.View(func(state agentstore.State) error {
		if state.Tasks[task.ID].Task.Status != "succeeded" || state.Tasks[task.ID].Task.Budget.DispatchesConsumed != 1 {
			t.Fatal("persisted result missing")
		}
		return nil
	})
}

func TestOfficialPrintPathsWithFakeProcesses(t *testing.T) {
	for _, adapter := range []string{"cursor", "antigravity"} {
		t.Run(adapter, func(t *testing.T) {
			s, _ := fixture(t)
			p := s.Profiles[0]
			body, e := os.ReadFile(p.Binary)
			if e != nil {
				t.Fatal(e)
			}
			body = []byte(strings.Replace(string(body), "export XCLI_NAG_HELPER=1", "export XCLI_NAG_HELPER=1\nexport XCLI_NAG_ADAPTER="+adapter, 1))
			if e = os.WriteFile(p.Binary, body, 0o700); e != nil {
				t.Fatal(e)
			}
			p.BinarySHA256 = api.Hash(body)
			p.AdapterID = "cursor-headless-v1"
			if adapter == "antigravity" {
				p.AdapterID = "antigravity-print-v1"
			}
			b := nativeadapter.BoundRequest{TaskID: api.ID("tsk"), SessionID: api.ID("ses"), Operation: "text_task", Prompt: "print", Workspace: t.TempDir(), Profile: p}
			r := (nativeadapter.Exec{}).StartTurn(context.Background(), b, nil)
			if r.Code != "" || r.Terminal != "success" || !r.SessionObserved || r.NativeID != "fixture-print" || r.Text != "fixture result" {
				t.Fatal(r)
			}
			b.NativeID = r.NativeID
			r = (nativeadapter.Exec{}).StartTurn(context.Background(), b, nil)
			if r.NativeID != b.NativeID || r.Code != "" {
				t.Fatal(r)
			}
			b.Prompt = "trust"
			want := "workspace_untrusted"
			if adapter == "antigravity" {
				b.Prompt = "soft-deny"
				want = "approval_required"
			}
			r = (nativeadapter.Exec{}).StartTurn(context.Background(), b, nil)
			if r.Code != want {
				t.Fatal(r)
			}
		})
	}
}
func TestObservedBinaryDriftDisablesCachedProfileWithoutSpawn(t *testing.T) {
	s, c := fixture(t)
	ses := newSession(t, s, c)
	out := create(t, s, c, ses, "first", 10)
	if e := os.WriteFile(s.Profiles[0].Binary, []byte("#!/bin/sh\nexit 13\n"), 0o700); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ExecuteNext(context.Background(), nativeadapter.Exec{}); e != nil {
		t.Fatal(e)
	}
	out, _ = s.GetTask(c, out.ID)
	if out.Status != "blocked" || out.Error.Code != "profile_changed" || out.Budget.DispatchesConsumed != 0 {
		t.Fatal(out)
	}
	caps, e := s.Capabilities(c)
	if e != nil || caps.Agents[0].Operations["text_task"].Availability != "blocked" {
		t.Fatal(caps, e)
	}
}
func TestJournalFailureDuringActivityStopsFakeNativeWorker(t *testing.T) {
	s, c := fixture(t)
	ses := newSession(t, s, c)
	_ = create(t, s, c, ses, "sleep", 10)
	count := 0
	s.Store.BeforeSync = func() error {
		count++
		if count >= 2 {
			return syscall.ENOSPC
		}
		return nil
	}
	start := time.Now()
	if _, e := s.ExecuteNext(context.Background(), nativeadapter.Exec{}); e == nil || time.Since(start) > 6*time.Second {
		t.Fatal("journal error did not stop worker", e, time.Since(start))
	}
}

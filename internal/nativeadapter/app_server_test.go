package nativeadapter_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/Garden12138/xcli/internal/nativeadapter"
)

// Executable fake JSON-RPC peer. No installed CLI, auth or model is used.
func TestAppServerHelperProcess(t *testing.T) {
	index := -1
	for i, arg := range os.Args {
		if arg == "--" {
			index = i
			break
		}
	}
	if index < 0 || index+2 >= len(os.Args) || os.Args[index+1] != "xcli-app-server-fixture" {
		return
	}
	mode, ledger := os.Args[index+2], os.Args[index+3]
	thread, turn := "thr_fixture_private", "turn_fixture"
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	send := func(v any) { b, _ := json.Marshal(v); fmt.Println(string(b)) }
	reply := func(id json.RawMessage, v any) { send(map[string]any{"id": id, "result": v}) }
	notify := func(method string, params any) { send(map[string]any{"method": method, "params": params}) }
	output := func(status string) {
		notify("item/agentMessage/delta", map[string]any{"threadId": thread, "turnId": turn, "itemId": "msg_1", "delta": "synthetic "})
		notify("item/completed", map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"id": "msg_1", "type": "agentMessage", "text": "synthetic app-server result", "phase": "final_answer"}})
		notify("turn/completed", map[string]any{"threadId": thread, "turn": map[string]any{"id": turn, "status": status}})
	}
	for scanner.Scan() {
		var m struct {
			ID            json.RawMessage
			Method        string
			Params        map[string]json.RawMessage
			Result, Error json.RawMessage
		}
		if json.Unmarshal(scanner.Bytes(), &m) != nil {
			os.Exit(90)
		}
		f, _ := os.OpenFile(ledger, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		name := m.Method
		if name == "" {
			name = "server-request-reply:" + string(m.Result) + string(m.Error)
		}
		fmt.Fprintln(f, name)
		f.Close()
		switch m.Method {
		case "initialize":
			reply(m.ID, map[string]any{"userAgent": "fake-no-model"})
		case "initialized":
		case "thread/start", "thread/resume":
			if m.Method == "thread/resume" {
				_ = json.Unmarshal(m.Params["threadId"], &thread)
				if mode == "lost" {
					thread = "different_thread"
				}
			}
			reply(m.ID, map[string]any{"thread": map[string]any{"id": thread}})
		case "turn/start":
			// Every outgoing turn must carry the server-owned permissions.
			var approval string
			json.Unmarshal(m.Params["approvalPolicy"], &approval)
			var policy map[string]any
			json.Unmarshal(m.Params["sandboxPolicy"], &policy)
			if approval != "never" || policy["type"] != "readOnly" {
				os.Exit(91)
			}
			if mode == "malformed" {
				fmt.Println(`{"id":1,"id":2,"result":{}}`)
				continue
			}
			if mode == "wrong-response" {
				reply(json.RawMessage(`9999`), map[string]any{})
				continue
			}
			if mode == "quota" {
				send(map[string]any{"id": m.ID, "error": map[string]any{"code": -32000, "data": map[string]any{"codexErrorInfo": "usageLimitExceeded"}}})
				continue
			}
			if mode == "early" {
				output("completed")
			}
			reply(m.ID, map[string]any{"turn": map[string]any{"id": turn, "status": "inProgress"}})
			switch mode {
			case "sleep", "late", "interrupt-ack":
			case "approval", "permissions", "unknown-request":
				method := "item/commandExecution/requestApproval"
				if mode == "permissions" {
					method = "item/permissions/requestApproval"
				}
				if mode == "unknown-request" {
					method = "account/chatgptAuthTokens/refresh"
				}
				send(map[string]any{"id": "srv_approval", "method": method, "params": map[string]any{"threadId": thread, "turnId": turn, "itemId": "tool_1"}})
			case "early":
			case "noterminal":
				os.Exit(0)
			case "crash":
				os.Exit(17)
			case "partial-quota":
				notify("item/completed", map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"id": "partial_1", "type": "agentMessage", "text": "partial synthetic text", "phase": "partial_answer"}})
				notify("error", map[string]any{"threadId": thread, "turnId": turn, "error": map[string]any{"codexErrorInfo": "usageLimitExceeded"}, "willRetry": true})
			case "wrong-thread":
				thread = "other_thread"
				output("completed")
			case "overlong":
				fmt.Printf("{\"method\":\"irrelevant\",\"params\":{\"data\":\"%s\"}}\n", strings.Repeat("x", 2<<20))
			default:
				output("completed")
			}
		case "turn/interrupt":
			reply(m.ID, map[string]any{})
			if mode == "late" {
				output("completed")
			} else if mode != "interrupt-ack" {
				output("interrupted")
			}
		case "":
		default:
			os.Exit(92)
		}
	}
	os.Exit(0)
}
func appFixture(t *testing.T, mode string) (*agentservice.Service, agentstore.Client, string, string) {
	t.Helper()
	old, c := fixture(t)
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	ledger := filepath.Join(t.TempDir(), "methods")
	script := filepath.Join(t.TempDir(), "fake-app-server")
	body := []byte("#!/bin/sh\nexport GORACE=atexit_sleep_ms=0\nexec " + quote(exe) + " -test.run=^TestAppServerHelperProcess$ -- xcli-app-server-fixture " + quote(mode) + " " + quote(ledger) + " \"$@\"\n")
	if e = os.WriteFile(script, body, 0700); e != nil {
		t.Fatal(e)
	}
	p := old.Profiles[0]
	p.AgentID = "codex"
	p.AdapterID = agentpolicy.AppServerAdapter
	p.PolicyID = agentpolicy.AppServerPolicy
	p.Binary = script
	p.BinarySHA256 = api.Hash(body)
	p.SHA256 = ""
	p.CapabilityRevision = ""
	p.AppServer = &agentpolicy.AppServerBinding{Root: old.Store.Root}
	c.Agents = []string{"codex"}
	c.Policies = []string{p.PolicyID}
	c.ID = "app-server-fixture"
	token, e := old.Clients.Issue(c, false)
	if e != nil {
		t.Fatal(e)
	}
	c, e = old.Clients.Get(c.ID)
	if e != nil {
		t.Fatal(e)
	}
	cfg := old.Config
	cfg.CodexAppServerEnabled = true
	s, e := agentservice.New(old.Store, old.Clients, cfg, []agentpolicy.Profile{p})
	if e != nil {
		t.Fatal(e)
	}
	return s, c, token, ledger
}
func TestAppServerHTTPFakePersistenceIdempotencyAndResume(t *testing.T) {
	s, c, token, ledger := appFixture(t, "early")
	serverHandler := &agentgateway.Server{Service: s}
	server := httptest.NewServer(serverHandler)
	defer server.Close()
	serverHandler.Address = strings.TrimPrefix(server.URL, "http://")
	request := func(method, path, key string, body any) (int, []byte) {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", key)
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, b
	}
	p := s.Profiles[0]
	status, data := request("POST", "/agent/v1/sessions", "app-session", api.SessionCreate{AgentID: p.AgentID, AdapterID: p.AdapterID, PolicyID: p.PolicyID, CapabilityRevision: p.CapabilityRevision, Budget: api.Budget{MaxDispatches: 2, MaxProcessStarts: 2, MaxExecutionSeconds: 300}, Context: api.Context{App: "worklog", Workflow: "g6-fake", OpaqueRef: "synthetic"}})
	if status != 201 {
		t.Fatal(status, string(data))
	}
	for _, field := range []string{"method", "native_thread_id", "native_args", "env", "cwd", "sandboxPolicy"} {
		payload := map[string]any{"session_id": "fake", "expected_session_revision": 1, "operation": "text_task", "input": []any{map[string]any{"type": "text", "text": "synthetic"}}, "budget": map[string]any{"max_dispatches": 1, "max_process_starts": 1, "timeout_seconds": 120}, "consent": map[string]any{"allow_remote": true, "context_sha256": "fake"}, field: "untrusted"}
		code, _ := request("POST", "/agent/v1/tasks", "malicious-"+field, payload)
		if code != 422 {
			t.Fatal("native parameter accepted", field, code)
		}
	}
	for _, path := range []string{"rpc", "command/exec", "account/login/start", "config/value/write"} {
		code, _ := request("POST", "/agent/v1/"+path, "malicious-route", map[string]any{})
		if code != 404 {
			t.Fatal(path, code)
		}
	}
	var ses api.Session
	json.Unmarshal(data, &ses)
	text := "fixed synthetic task"
	req := api.TaskCreate{SessionID: ses.ID, ExpectedSessionRevision: ses.Revision, Operation: "text_task", Input: []api.Part{{Type: "text", Text: &text}}, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, TimeoutSeconds: 10}, Consent: api.Consent{AllowRemote: true, ContextSHA256: ses.ContextSHA256}}
	var wg sync.WaitGroup
	ids := make(chan string, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, data := request("POST", "/agent/v1/tasks", "app-one-task", req)
			if status == 429 {
				return
			} // Bounded HTTP admission, not a dispatch.
			if status != 202 {
				t.Error(status)
				return
			}
			var task api.Task
			json.Unmarshal(data, &task)
			ids <- task.ID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for next := range ids {
		if id != "" && next != id {
			t.Fatal("duplicate task")
		}
		id = next
	}
	if id == "" {
		t.Fatal("no request admitted")
	}
	if _, e := s.ExecuteNext(context.Background(), nativeadapter.Exec{}); e != nil {
		t.Fatal(e)
	}
	status, data = request("GET", "/agent/v1/tasks/"+id, "", nil)
	var task api.Task
	json.Unmarshal(data, &task)
	if status != 200 || task.Status != "succeeded" || task.Result.Text == nil || *task.Result.Text != "synthetic app-server result" {
		t.Fatal(status, string(data))
	}
	status, events := request("GET", "/agent/v1/tasks/"+id+"/events", "", nil)
	if status != 200 || !bytes.Contains(events, []byte("text.completed")) || !bytes.Contains(events, []byte("task.completed")) || bytes.Contains(data, []byte("thr_fixture_private")) {
		t.Fatal("SSE or native ID exposure")
	}
	other := c
	other.ID = "other"
	if _, e := s.GetSession(other, ses.ID); e == nil {
		t.Fatal("cross-client thread takeover")
	}
	root := s.Store.Root
	if e := s.Store.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := agentstore.Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	// Rebuild from the same registered configuration, not runtime capability
	// snapshots (whose hashes include their generated capability revision).
	registered := s.Profiles[0]
	registered.SHA256 = ""
	registered.CapabilityRevision = ""
	resumed, e := agentservice.New(reopened, s.Clients, s.Config, []agentpolicy.Profile{registered})
	if e != nil {
		t.Fatal(e)
	}
	restored, e := resumed.GetTask(c, id)
	if e != nil || restored.Status != "succeeded" {
		t.Fatal(e, restored)
	}
	ses, e = resumed.GetSession(c, ses.ID)
	if e != nil {
		t.Fatal(e)
	}
	next := create(t, resumed, c, ses, "second", 10)
	if _, e = resumed.ExecuteNext(context.Background(), nativeadapter.Exec{}); e != nil {
		t.Fatal(e)
	}
	result, e := resumed.GetTask(c, next.ID)
	if e != nil || result.Status != "succeeded" {
		t.Fatal(result, e)
	}
	calls, _ := os.ReadFile(ledger)
	if strings.Count(string(calls), "turn/start\n") != 2 || !strings.Contains(string(calls), "thread/resume") {
		t.Fatal(string(calls))
	}
}
func TestAppServerProtocolFailuresAndDenyOnly(t *testing.T) {
	for mode, code := range map[string]string{"approval": "approval_required", "permissions": "approval_required", "unknown-request": "native_auth_required", "malformed": "protocol_error", "wrong-response": "protocol_error", "wrong-thread": "protocol_error", "noterminal": "protocol_error", "overlong": "output_limit", "quota": "native_quota_exceeded", "partial-quota": "native_quota_exceeded", "crash": "native_process_failed"} {
		t.Run(mode, func(t *testing.T) {
			s, c, _, ledger := appFixture(t, mode)
			ses := newSession(t, s, c)
			task := create(t, s, c, ses, "synthetic", 10)
			if _, e := s.ExecuteNext(context.Background(), nativeadapter.Exec{}); e != nil {
				t.Fatal(e)
			}
			out, e := s.GetTask(c, task.ID)
			if e != nil || out.Result.Final || out.Status == "succeeded" || out.Error == nil || out.Error.Code != code {
				t.Fatalf("%s %+v %v", mode, out, e)
			}
			calls, _ := os.ReadFile(ledger)
			if strings.Contains(string(calls), `"accept"`) || strings.Contains(string(calls), "account/login") {
				t.Fatal(string(calls))
			}
			if mode == "approval" && !strings.Contains(string(calls), `"decline"`) {
				t.Fatal("denial reply not received", string(calls))
			}
			if mode == "partial-quota" && (out.Result.Text == nil || *out.Result.Text != "partial synthetic text") {
				t.Fatal("partial text lost", out)
			}
		})
	}
}

func TestAppServerDisabledAndNativeFailBeforeSpawn(t *testing.T) {
	for _, native := range []bool{false, true} {
		s, c, _, ledger := appFixture(t, "success")
		p := s.Profiles[0]
		p.Native = native
		p.Blocker = ""
		cfg := s.Config
		if !native {
			cfg.CodexAppServerEnabled = false
		}
		blocked, e := agentservice.New(s.Store, s.Clients, cfg, []agentpolicy.Profile{p})
		if e != nil {
			t.Fatal(e)
		}
		p = blocked.Profiles[0]
		_, _, e = blocked.CreateSession(c, api.SessionCreate{AgentID: p.AgentID, AdapterID: p.AdapterID, PolicyID: p.PolicyID, CapabilityRevision: p.CapabilityRevision, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, MaxExecutionSeconds: 120}, Context: api.Context{App: "worklog", Workflow: "synthetic"}}, api.ID("key"))
		if e == nil {
			t.Fatal("disabled/native profile promoted")
		}
		if _, e = os.Stat(ledger); !os.IsNotExist(e) {
			t.Fatal("spawned before safety gate")
		}
	}
}
func TestAppServerCancelTimeoutRevokeDisableAndLateSuccess(t *testing.T) {
	for _, mode := range []string{"cancel", "timeout", "revoke", "disable", "late", "interrupt-ack"} {
		t.Run(mode, func(t *testing.T) {
			peer := "sleep"
			if mode == "late" || mode == "interrupt-ack" {
				peer = mode
			}
			s, c, _, ledger := appFixture(t, peer)
			ses := newSession(t, s, c)
			timeout := 10
			if mode == "timeout" {
				timeout = 1
			}
			task := create(t, s, c, ses, "synthetic", timeout)
			finished := make(chan error, 1)
			go func() { _, e := s.ExecuteNext(context.Background(), nativeadapter.Exec{}); finished <- e }()
			until := time.Now().Add(5 * time.Second)
			for {
				data, _ := os.ReadFile(ledger)
				if strings.Contains(string(data), "turn/start") {
					break
				}
				if time.Now().After(until) {
					t.Fatal("no fake turn")
				}
				time.Sleep(5 * time.Millisecond)
			}
			// Let the start response arrive, so interrupt's turn ID is authoritative.
			time.Sleep(25 * time.Millisecond)
			switch mode {
			case "cancel", "late", "interrupt-ack":
				_, _, e := s.Cancel(c, task.ID, api.ID("key"))
				if e != nil {
					t.Fatal(e)
				}
			case "revoke":
				if e := s.Clients.Revoke(c.ID); e != nil {
					t.Fatal(e)
				}
			case "disable":
				if e := agentpolicy.DisableAppServer(s.Store.Root); e != nil {
					t.Fatal(e)
				}
			}
			select {
			case e := <-finished:
				if e != nil {
					t.Fatal(e)
				}
			case <-time.After(7 * time.Second):
				t.Fatal("cleanup unbounded")
			}
			var out api.Task
			s.Store.View(func(st agentstore.State) error { out = st.Tasks[task.ID].Task; return nil })
			if out.Status == "succeeded" || out.Result.Final || out.Execution.LocalStopConfirmed == nil || !*out.Execution.LocalStopConfirmed {
				t.Fatal(out)
			}
			if mode == "timeout" && out.Status != "timed_out" {
				t.Fatal(out)
			}
			if mode == "interrupt-ack" && out.Execution.UpstreamCancelConfirmed != nil {
				t.Fatal("ACK became upstream confirmation")
			}
			if mode == "cancel" && (out.Execution.NativeTerminal != "interrupted" || out.Execution.UpstreamCancelConfirmed != nil) {
				t.Fatal("native interruption missing or confused with cloud cancellation")
			}
			if mode == "disable" && s.Profiles[0].Check("text_task") == nil {
				t.Fatal("disabled profile admitted new task")
			}
		})
	}
}

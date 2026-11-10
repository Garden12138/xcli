package nativeadapter_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentgateway"
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/agentservice"
	"github.com/Garden12138/xcli/internal/agentstore"
	"github.com/Garden12138/xcli/internal/isolatedruntime"
	"github.com/Garden12138/xcli/internal/nativeadapter"
)

func TestIsolatedTransportDefaultClosed(t *testing.T) {
	result := (nativeadapter.IsolatedAppServer{}).StartTurn(context.Background(), nativeadapter.BoundRequest{}, nil)
	if result.Code != "policy_unenforceable" || !result.LocalStopConfirmed || result.ExitCode != nil {
		t.Fatal(result)
	}
}

// Explicit external-system test: default offline runs skip it. The opt-in run
// uses only the cross-built g7_peer fixture, never the real codex executable.
func TestG7ContainerHTTPFakePeerAndRuntimeLoss(t *testing.T) {
	if os.Getenv("XCLI_G7_RUN_CONTAINER_TEST") != "1" {
		t.Skip("explicit fake-peer container experiment required")
	}
	ws, e := filepath.EvalSymlinks(os.Getenv("XCLI_G7_WORKSPACE"))
	if e != nil {
		t.Fatal(e)
	}
	executable := filepath.Join(ws, "g7-fake-peer")
	payload, e := os.ReadFile(executable)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.HasPrefix(payload, []byte{0x7f, 'E', 'L', 'F'}) || !bytes.Contains(payload, []byte("G7_FAKE_NO_MODEL")) {
		t.Fatal("explicit Linux fake peer required")
	}
	docker := "/usr/local/bin/docker"
	config, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	os.Chmod(config, 0700)
	var nonce [16]byte
	rand.Read(nonce[:])
	runtime, e := isolatedruntime.NewDocker(isolatedruntime.DockerOptions{Binary: docker, Endpoint: os.Getenv("XCLI_G7_SOCKET"), ConfigDir: config, Workspace: ws, Image: os.Getenv("XCLI_G7_IMAGE"), Executable: "g7-fake-peer", Nonce: hex.EncodeToString(nonce[:])})
	if e != nil {
		t.Fatal(e)
	}
	driver := nativeadapter.IsolatedAppServer{Enabled: true, Runtime: runtime}
	root := filepath.Join(t.TempDir(), "state")
	store, e := agentstore.Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { store.Close() }()
	clients := &agentstore.Clients{Root: store.Root}
	client := agentstore.Client{ID: "g7-fake-client", Scopes: []string{"session:create", "session:read", "session:close", "task:create", "task:read", "task:cancel"}, Agents: []string{"codex"}, Policies: []string{agentpolicy.AppServerPolicy}, Operations: []string{"text_task"}, AllowRemote: true, DailyDispatches: 2, DailyProcessStarts: 2, DailyExecutionSeconds: 240}
	token, e := clients.Issue(client, false)
	if e != nil {
		t.Fatal(e)
	}
	client, e = clients.Get(client.ID)
	if e != nil {
		t.Fatal(e)
	}
	cfg := agentpolicy.DefaultConfig()
	cfg.CodexAppServerEnabled = true
	cfg.DailyDispatches = 2
	cfg.DailyProcessStarts = 2
	cfg.DailyExecutionSeconds = 240
	registered := agentpolicy.Profile{AgentID: "codex", AdapterID: agentpolicy.AppServerAdapter, PolicyID: agentpolicy.AppServerPolicy, Revision: "g7-fake-only", InstallationDomain: "g7-fake-only", Location: "remote", Binary: executable, BinarySHA256: api.Hash(payload), Operations: []string{"text_task"}, AppServer: &agentpolicy.AppServerBinding{Root: store.Root}}
	service, e := agentservice.New(store, clients, cfg, []agentpolicy.Profile{registered})
	if e != nil {
		t.Fatal(e)
	}
	serverHandler := &agentgateway.Server{Service: service}
	server := httptest.NewServer(serverHandler)
	serverHandler.Address = strings.TrimPrefix(server.URL, "http://")
	defer server.Close()
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
		var output bytes.Buffer
		output.ReadFrom(res.Body)
		return res.StatusCode, output.Bytes()
	}
	outcomes := []map[string]any{}
	for index, scenario := range []string{"success", "destroy"} {
		p := service.Profiles[0]
		key := "g7-container-" + scenario
		code, data := request("POST", "/agent/v1/sessions", key+"-session", api.SessionCreate{AgentID: p.AgentID, AdapterID: p.AdapterID, PolicyID: p.PolicyID, CapabilityRevision: p.CapabilityRevision, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, MaxExecutionSeconds: 120}, Context: api.Context{App: "worklog", Workflow: "g7-fake-only", OpaqueRef: scenario}})
		if code != 201 {
			t.Fatal(code, string(data))
		}
		var session api.Session
		json.Unmarshal(data, &session)
		text := "g7_fake_success"
		if scenario == "destroy" {
			text = "g7_fake_wait"
		}
		body := api.TaskCreate{SessionID: session.ID, ExpectedSessionRevision: session.Revision, Operation: "text_task", Input: []api.Part{{Type: "text", Text: &text}}, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, TimeoutSeconds: 120}, Consent: api.Consent{AllowRemote: true, ContextSHA256: session.ContextSHA256}}
		code, data = request("POST", "/agent/v1/tasks", key+"-task", body)
		if code != 202 {
			t.Fatal(code, string(data))
		}
		var task api.Task
		json.Unmarshal(data, &task)
		finished := make(chan error, 1)
		go func() { _, e := service.ExecuteNext(context.Background(), driver); finished <- e }()
		if scenario == "destroy" {
			deadline := time.Now().Add(7 * time.Second)
			for {
				events, _, _, e := service.Events(client, task.ID, 0)
				encoded, _ := json.Marshal(events)
				if e == nil && bytes.Contains(encoded, []byte(`"category":"session"`)) {
					break
				}
				if time.Now().After(deadline) {
					runtime.StopConfirmed(task.ID)
					t.Fatal("fake container did not bind")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !runtime.StopConfirmed(task.ID) {
				t.Fatal("owned runtime destruction unconfirmed")
			}
		}
		select {
		case e := <-finished:
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(8 * time.Second):
			runtime.StopConfirmed(task.ID)
			t.Fatal("driver did not converge")
		}
		code, data = request("GET", "/agent/v1/tasks/"+task.ID, "", nil)
		var result api.Task
		json.Unmarshal(data, &result)
		if code != 200 || result.Budget.ProcessStartsConsumed != 1 || result.Execution.LocalStopConfirmed == nil || !*result.Execution.LocalStopConfirmed {
			t.Fatal(string(data))
		}
		if scenario == "success" && (result.Status != "succeeded" || result.Result.Text == nil || *result.Result.Text != "G7_FAKE_SUCCESS") {
			t.Fatal(string(data))
		}
		if scenario == "destroy" && (result.Status == "succeeded" || result.Result.Final) {
			t.Fatal("destroyed runtime succeeded", string(data))
		}
		eventCode, events := request("GET", "/agent/v1/tasks/"+task.ID+"/events", "", nil)
		if eventCode != 200 || !bytes.Contains(events, []byte("task.")) {
			t.Fatal("SSE missing")
		}
		replayCode, replayed := request("POST", "/agent/v1/tasks", key+"-task", body)
		var repeated api.Task
		json.Unmarshal(replayed, &repeated)
		if replayCode != 202 || repeated.ID != task.ID {
			t.Fatal("idempotent retry created another task")
		}
		if ran, e := service.ExecuteNext(context.Background(), driver); e != nil || ran {
			t.Fatal("automatic retry", e)
		}
		outcomes = append(outcomes, map[string]any{"scenario": scenario, "status": result.Status, "process_starts": result.Budget.ProcessStartsConsumed, "local_stop_confirmed": true, "cloud_stop_confirmed": result.Execution.UpstreamCancelConfirmed, "fake_peer_only": true})
		if index == 0 {
			continue
		}
	}
	server.Close()
	if e = store.Close(); e != nil {
		t.Fatal(e)
	}
	store, e = agentstore.Open(root)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := agentservice.New(store, clients, cfg, []agentpolicy.Profile{registered})
	if e != nil {
		t.Fatal(e)
	}
	if ran, e := recovered.ExecuteNext(context.Background(), driver); e != nil || ran {
		t.Fatal("restart re-executed task", e)
	}
	t.Logf("G7_A5_EVIDENCE %s", mustJSON(map[string]any{"outcomes": outcomes, "restart_no_reexecution": true, "real_model_requests": 0, "fake_binary_sha256": api.Hash(payload)}))
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
